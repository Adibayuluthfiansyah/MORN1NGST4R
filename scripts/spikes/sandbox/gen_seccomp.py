#!/usr/bin/env python3
"""Hand-assembled classic-BPF seccomp filter generator (x86_64 only, no dependencies).

Usage:
  gen_seccomp.py --profile production --out FILE      write sock_filter array for `bwrap --seccomp FD`
  gen_seccomp.py --profile production --dump          print a readable listing
  gen_seccomp.py --profile deny-userns --exec -- CMD  install the filter in this process, then exec CMD
                                                       (used to simulate "user namespaces denied")

Profiles
  production   deny ptrace, bpf, io_uring_*, keyctl family, perf_event_open, mount family,
               new user namespaces (unshare/clone with CLONE_NEWUSER; clone3 -> ENOSYS so libc falls
               back to clone), setns, process_vm_*, userfaultfd, module/kexec/reboot/swap, TIOCSTI/TIOCLINUX.
               Everything else is allowed. Wrong architecture (anything but x86_64) is killed.
  deny-userns  only the CLONE_NEWUSER denial (test helper)
  deny-newnet  only the CLONE_NEWNET denial (test helper, simulates "network namespaces unavailable")

Return value for denied calls is EPERM, except clone3 (ENOSYS, deliberate) and wrong-arch (KILL_PROCESS).
"""
import ctypes
import struct
import sys

AUDIT_ARCH_X86_64 = 0xC000003E
EPERM, ENOSYS = 1, 38
SECCOMP_RET_ALLOW = 0x7FFF0000
SECCOMP_RET_KILL_PROCESS = 0x80000000
SECCOMP_RET_ERRNO = 0x00050000
CLONE_NEWUSER = 0x10000000
CLONE_NEWNET = 0x40000000
TIOCSTI, TIOCLINUX = 0x5412, 0x541C
X32_BIT = 0x40000000

# x86_64 syscall numbers (arch/x86/entry/syscalls/syscall_64.tbl). FRAGILE: valid for x86_64 only.
DENY = {
    "ptrace": 101, "bpf": 321, "io_uring_setup": 425, "io_uring_enter": 426, "io_uring_register": 427,
    "keyctl": 250, "add_key": 248, "request_key": 249, "perf_event_open": 298,
    "mount": 165, "umount2": 166, "pivot_root": 155, "chroot": 161,
    "open_tree": 428, "move_mount": 429, "fsopen": 430, "fsconfig": 431, "fsmount": 432, "fspick": 433,
    "mount_setattr": 442, "setns": 308, "process_vm_readv": 310, "process_vm_writev": 311,
    "userfaultfd": 323, "kexec_load": 246, "kexec_file_load": 320, "init_module": 175,
    "finit_module": 313, "delete_module": 176, "reboot": 169, "swapon": 167, "swapoff": 168,
}
NR_CLONE, NR_UNSHARE, NR_CLONE3, NR_IOCTL = 56, 272, 435, 16

# BPF opcodes
LD_W_ABS = 0x20   # BPF_LD | BPF_W | BPF_ABS
JEQ_K = 0x15      # BPF_JMP | BPF_JEQ | BPF_K
JGE_K = 0x35
JSET_K = 0x45
RET_K = 0x06


class Asm:
    def __init__(self):
        self.ins = []      # (code, jt_label|int, jf_label|int, k)
        self.labels = {}

    def label(self, name):
        self.labels[name] = len(self.ins)

    def ld(self, off):
        self.ins.append((LD_W_ABS, 0, 0, off))

    def jmp(self, code, k, jt, jf):
        self.ins.append((code, jt, jf, k))

    def ret(self, k):
        self.ins.append((RET_K, 0, 0, k))

    def assemble(self):
        out = []
        for i, (code, jt, jf, k) in enumerate(self.ins):
            def rel(t):
                if isinstance(t, int):
                    return t
                d = self.labels[t] - (i + 1)
                if not 0 <= d <= 255:
                    raise ValueError("jump out of range")
                return d
            out.append(struct.pack("=HBBI", code, rel(jt), rel(jf), k))
        return b"".join(out)


def build(profile="production"):
    a = Asm()
    # arch check
    a.ld(4)
    a.jmp(JEQ_K, AUDIT_ARCH_X86_64, "arch_ok", "kill")
    a.label("arch_ok")
    a.ld(0)  # syscall number
    if profile == "production":
        a.jmp(JGE_K, X32_BIT, "eperm", "nox32")   # x32 ABI numbers
        a.label("nox32")
        for nr in sorted(DENY.values()):
            a.jmp(JEQ_K, nr, "eperm", 0)
        a.jmp(JEQ_K, NR_CLONE3, "enosys", 0)
        a.jmp(JEQ_K, NR_IOCTL, "ioctl", 0)
    a.jmp(JEQ_K, NR_CLONE, "flags0", 0)
    a.jmp(JEQ_K, NR_UNSHARE, "flags0", "allow")
    a.label("flags0")
    a.ld(16)  # args[0], low 32 bits (little endian): flags for clone/unshare
    a.jmp(JSET_K, CLONE_NEWNET if profile == "deny-newnet" else CLONE_NEWUSER, "eperm", "allow")
    if profile == "production":
        a.label("ioctl")
        a.ld(16 + 8)  # args[1] low word: ioctl request
        a.jmp(JEQ_K, TIOCSTI, "eperm", 0)
        a.jmp(JEQ_K, TIOCLINUX, "eperm", "allow")
    a.label("allow")
    a.ret(SECCOMP_RET_ALLOW)
    a.label("eperm")
    a.ret(SECCOMP_RET_ERRNO | EPERM)
    a.label("enosys")
    a.ret(SECCOMP_RET_ERRNO | ENOSYS)
    a.label("kill")
    a.ret(SECCOMP_RET_KILL_PROCESS)
    return a.assemble()


def install(prog):
    PR_SET_NO_NEW_PRIVS, PR_SET_SECCOMP, SECCOMP_MODE_FILTER = 38, 22, 2
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != 0:
        raise OSError(ctypes.get_errno(), "PR_SET_NO_NEW_PRIVS")
    buf = ctypes.create_string_buffer(prog, len(prog))

    class Fprog(ctypes.Structure):
        _fields_ = [("len", ctypes.c_ushort), ("filter", ctypes.c_void_p)]
    fp = Fprog(len(prog) // 8, ctypes.cast(buf, ctypes.c_void_p))
    if libc.prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, ctypes.byref(fp)) != 0:
        raise OSError(ctypes.get_errno(), "PR_SET_SECCOMP")


def main(argv):
    import os
    profile, out, dump, do_exec, cmd = "production", None, False, False, []
    i = 0
    while i < len(argv):
        if argv[i] == "--profile":
            profile = argv[i + 1]; i += 2
        elif argv[i] == "--out":
            out = argv[i + 1]; i += 2
        elif argv[i] == "--dump":
            dump = True; i += 1
        elif argv[i] == "--exec":
            do_exec = True; i += 1
        elif argv[i] == "--":
            cmd = argv[i + 1:]; break
        else:
            print(__doc__, file=sys.stderr); return 2
    prog = build(profile)
    if dump:
        print(f"profile={profile} instructions={len(prog)//8} bytes={len(prog)}")
        for n in range(len(prog) // 8):
            c, jt, jf, k = struct.unpack_from("=HBBI", prog, n * 8)
            print(f"{n:3d}: code=0x{c:04x} jt={jt:3d} jf={jf:3d} k=0x{k:08x}")
    if out:
        with open(out, "wb") as f:
            f.write(prog)
    if do_exec:
        install(prog)
        os.execvp(cmd[0], cmd)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
