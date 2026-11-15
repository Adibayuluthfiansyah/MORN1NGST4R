/*
 * probe.c - static in-sandbox probe for the MORN1NGST4R sandbox feasibility spike.
 *
 * Build:  gcc -static -O2 -DPROBE_ID='"A"' -o probe probe.c
 * It performs only local, bounded, harmless actions. The only network
 * destinations it touches are 127.0.0.1, ::1 and the RFC 5737 address
 * 192.0.2.1 (to confirm that a sandbox has NO route).
 */
#define _GNU_SOURCE
#include <arpa/inet.h>
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <linux/capability.h>
#include <linux/landlock.h>
#include <netinet/in.h>
#include <poll.h>
#include <sched.h>
#include <termios.h>
#include <sys/stat.h>
#include <signal.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <stddef.h>
#include <sys/mount.h>
#include <sys/prctl.h>
#include <sys/ptrace.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

#ifndef PROBE_ID
#define PROBE_ID "X"
#endif
static const char probe_id[] = "PROBE_ID_MARKER=" PROBE_ID;

#ifndef SYS_io_uring_setup
#define SYS_io_uring_setup 425
#define SYS_io_uring_enter 426
#define SYS_io_uring_register 427
#endif
#ifndef SYS_landlock_create_ruleset
#define SYS_landlock_create_ruleset 444
#define SYS_landlock_add_rule 445
#define SYS_landlock_restrict_self 446
#endif
#ifndef SYS_open_tree
#define SYS_open_tree 428
#define SYS_move_mount 429
#define SYS_fsopen 430
#define SYS_fsconfig 431
#define SYS_fsmount 432
#define SYS_fspick 433
#define SYS_mount_setattr 442
#endif
#ifndef SYS_clone3
#define SYS_clone3 435
#endif
#ifndef CLONE_NEWUSER
#define CLONE_NEWUSER 0x10000000
#endif

static const char *en(int e) {
  switch (e) {
#define E(x) case x: return #x;
    E(EPERM) E(ENOENT) E(ESRCH) E(EINTR) E(EIO) E(ENXIO) E(EBADF) E(EAGAIN)
    E(ENOMEM) E(EACCES) E(EFAULT) E(EBUSY) E(EEXIST) E(ENOTDIR) E(EISDIR)
    E(EINVAL) E(EMFILE) E(EFBIG) E(ENOSPC) E(EROFS) E(ENOSYS) E(ENETUNREACH)
    E(ECONNREFUSED) E(EADDRNOTAVAIL) E(EAFNOSUPPORT) E(ETIMEDOUT) E(EHOSTUNREACH)
    E(ENOTTY) E(ENODEV) E(EOPNOTSUPP) E(ECONNRESET) E(ENETDOWN) E(EINPROGRESS)
    E(ENOTCONN) E(EPROTONOSUPPORT) E(ENOBUFS) E(ENAMETOOLONG)
#undef E
  case 0: return "OK";
  default: { static char nb[16]; snprintf(nb, sizeof nb, "E%d", e); return nb; }
  }
}

static long ms_now(void) {
  struct timespec t;
  clock_gettime(CLOCK_MONOTONIC, &t);
  return t.tv_sec * 1000L + t.tv_nsec / 1000000L;
}

/* ---------------------------------------------------------------- sha256 */
typedef struct { uint32_t s[8]; uint64_t n; uint8_t b[64]; size_t l; } sha_t;
static const uint32_t K[64] = {
 0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
 0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
 0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
 0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
 0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
 0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
 0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
 0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2};
#define ROR(x,n) (((x)>>(n))|((x)<<(32-(n))))
static void sha_block(sha_t *c, const uint8_t *p) {
  uint32_t w[64], a,b,cc,d,e,f,g,h;
  for (int i = 0; i < 16; i++) w[i] = (uint32_t)p[4*i]<<24|p[4*i+1]<<16|p[4*i+2]<<8|p[4*i+3];
  for (int i = 16; i < 64; i++) {
    uint32_t s0 = ROR(w[i-15],7)^ROR(w[i-15],18)^(w[i-15]>>3);
    uint32_t s1 = ROR(w[i-2],17)^ROR(w[i-2],19)^(w[i-2]>>10);
    w[i] = w[i-16]+s0+w[i-7]+s1;
  }
  a=c->s[0];b=c->s[1];cc=c->s[2];d=c->s[3];e=c->s[4];f=c->s[5];g=c->s[6];h=c->s[7];
  for (int i = 0; i < 64; i++) {
    uint32_t S1 = ROR(e,6)^ROR(e,11)^ROR(e,25), ch = (e&f)^(~e&g);
    uint32_t t1 = h+S1+ch+K[i]+w[i];
    uint32_t S0 = ROR(a,2)^ROR(a,13)^ROR(a,22), mj = (a&b)^(a&cc)^(b&cc);
    uint32_t t2 = S0+mj;
    h=g;g=f;f=e;e=d+t1;d=cc;cc=b;b=a;a=t1+t2;
  }
  c->s[0]+=a;c->s[1]+=b;c->s[2]+=cc;c->s[3]+=d;c->s[4]+=e;c->s[5]+=f;c->s[6]+=g;c->s[7]+=h;
}
static void sha_init(sha_t *c) {
  static const uint32_t i[8] = {0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19};
  memcpy(c->s, i, sizeof i); c->n = 0; c->l = 0;
}
static void sha_upd(sha_t *c, const uint8_t *d, size_t n) {
  c->n += n;
  while (n--) { c->b[c->l++] = *d++; if (c->l == 64) { sha_block(c, c->b); c->l = 0; } }
}
/* simple, correct finalisation (the helper above is intentionally unused) */
static void sha_final(sha_t *c, char out[65]) {
  uint64_t bits = c->n * 8;
  uint8_t pad = 0x80, z = 0;
  sha_upd(c, &pad, 1);
  while (c->l != 56) sha_upd(c, &z, 1);
  uint8_t lb[8];
  for (int i = 0; i < 8; i++) lb[i] = (uint8_t)(bits >> (56 - 8*i));
  sha_upd(c, lb, 8);
  for (int i = 0; i < 8; i++) sprintf(out + 8*i, "%08x", c->s[i]);
  out[64] = 0;
}
static int sha_file(const char *path, char out[65]) {
  int fd = open(path, O_RDONLY);
  if (fd < 0) return -1;
  sha_t c; sha_init(&c);
  uint8_t buf[65536]; ssize_t n;
  while ((n = read(fd, buf, sizeof buf)) > 0) sha_upd(&c, buf, (size_t)n);
  close(fd);
  sha_final(&c, out);
  return 0;
}

/* ------------------------------------------------------------- subcommands */
static int cmd_status(void) {
  FILE *f = fopen("/proc/self/status", "r");
  if (!f) { printf("status: open failed %s\n", en(errno)); return 1; }
  char l[256];
  while (fgets(l, sizeof l, f))
    if (!strncmp(l,"Uid",3)||!strncmp(l,"Gid",3)||!strncmp(l,"Cap",3)||!strncmp(l,"NoNewPrivs",10)||
        !strncmp(l,"Seccomp",7)||!strncmp(l,"NSpid",5)) fputs(l, stdout);
  fclose(f);
  return 0;
}

static int is_num(const char *s) { for (; *s; s++) if (*s < '0' || *s > '9') return 0; return 1; }

static int cmd_env(const char *marker) {
  extern char **environ;
  int n = 0;
  for (char **e = environ; *e; e++) { printf("ENV %s\n", *e); n++; }
  printf("ENV_COUNT %d\n", n);
  DIR *d = opendir("/proc");
  struct dirent *de;
  int found = 0;
  while ((de = readdir(d))) {
    if (!is_num(de->d_name)) continue;
    char p[64], comm[64] = "?";
    snprintf(p, sizeof p, "/proc/%s/comm", de->d_name);
    FILE *cf = fopen(p, "r"); if (cf) { if (fgets(comm, sizeof comm, cf)) comm[strcspn(comm, "\n")] = 0; fclose(cf); }
    snprintf(p, sizeof p, "/proc/%s/environ", de->d_name);
    int fd = open(p, O_RDONLY);
    if (fd < 0) { printf("PID %s (%s) environ: %s\n", de->d_name, comm, en(errno)); continue; }
    char buf[8192]; ssize_t r, tot = 0; int hit = 0, ncount = 0;
    while ((r = read(fd, buf, sizeof buf - 1)) > 0) {
      buf[r] = 0; tot += r;
      for (ssize_t i = 0; i < r; i++) if (buf[i] == 0) ncount++;
      for (ssize_t i = 0; i + (ssize_t)strlen(marker) <= r; i++)
        if (!memcmp(buf + i, marker, strlen(marker))) hit = 1;
    }
    close(fd);
    printf("PID %s (%s) environ: readable bytes=%zd vars=%d marker_found=%d\n", de->d_name, comm, tot, ncount, hit);
    found += hit;
  }
  closedir(d);
  printf("MARKER_HITS %d\n", found);
  return 0;
}

static int cmd_pids(int argc, char **argv) {
  DIR *d = opendir("/proc");
  struct dirent *de;
  printf("VISIBLE_PIDS");
  while ((de = readdir(d))) if (is_num(de->d_name)) printf(" %s", de->d_name);
  printf("\n");
  closedir(d);
  printf("SELF_PID %d\n", getpid());
  for (int i = 2; i < argc; i++) {
    int pid = atoi(argv[i]);
    int r = kill(pid, 0); int e = errno;
    printf("HOSTPID %d kill(0): %s\n", pid, r == 0 ? "OK(visible!)" : en(e));
    char p[64];
    snprintf(p, sizeof p, "/proc/%d/environ", pid);
    int fd = open(p, O_RDONLY); printf("HOSTPID %d open(environ): %s\n", pid, fd >= 0 ? "OK(READABLE!)" : en(errno)); if (fd >= 0) close(fd);
    snprintf(p, sizeof p, "/proc/%d/mem", pid);
    fd = open(p, O_RDONLY); printf("HOSTPID %d open(mem): %s\n", pid, fd >= 0 ? "OK(READABLE!)" : en(errno)); if (fd >= 0) close(fd);
  }
  return 0;
}

/* try to read /proc/<pid>/environ and /proc/<pid>/mem for an in-sandbox pid (e.g. 1) */
static int cmd_peek(int argc, char **argv) {
  for (int i = 2; i < argc; i++) {
    char p[64]; int fd;
    snprintf(p, sizeof p, "/proc/%s/environ", argv[i]);
    fd = open(p, O_RDONLY);
    if (fd < 0) printf("PEEK %s environ: %s\n", argv[i], en(errno));
    else { char b[64]; ssize_t r = read(fd, b, sizeof b); printf("PEEK %s environ: opened, read=%zd (%s)\n", argv[i], r, r < 0 ? en(errno) : "ok"); close(fd); }
    snprintf(p, sizeof p, "/proc/%s/mem", argv[i]);
    fd = open(p, O_RDONLY);
    if (fd < 0) printf("PEEK %s mem: %s\n", argv[i], en(errno));
    else { char b[16]; ssize_t r = pread(fd, b, sizeof b, 0x400000); printf("PEEK %s mem: opened, pread=%zd (%s)\n", argv[i], r, r < 0 ? en(errno) : "ok"); close(fd); }
  }
  return 0;
}

static int cmd_proc(void) {
  char b[256]; ssize_t n;
  n = readlink("/proc/self/ns/pid", b, sizeof b - 1); if (n > 0) { b[n] = 0; printf("NS pid %s\n", b); }
  const char *k[] = {"user","net","ipc","uts","cgroup","mnt"};
  for (int i = 0; i < 6; i++) { char p[64]; snprintf(p, sizeof p, "/proc/self/ns/%s", k[i]); n = readlink(p, b, sizeof b - 1); if (n > 0) { b[n] = 0; printf("NS %s %s\n", k[i], b); } }
  FILE *f = fopen("/proc/self/mountinfo", "r"); char l[512];
  while (f && fgets(l, sizeof l, f)) { if (strstr(l, " /proc ") || strstr(l, " /sys") || strstr(l, " /dev")) printf("MNT %s", l); }
  if (f) fclose(f);
  struct stat st; printf("STAT /sys: %s\n", stat("/sys", &st) == 0 ? "EXISTS" : en(errno));
  printf("STAT /sys/class: %s\n", stat("/sys/class", &st) == 0 ? "EXISTS" : en(errno));
  printf("STAT /proc/1/comm: ");
  f = fopen("/proc/1/comm", "r"); if (f) { if (fgets(l, sizeof l, f)) fputs(l, stdout); fclose(f); } else printf("%s\n", en(errno));
  return 0;
}

static int cmd_paths(int argc, char **argv) {
  for (int i = 2; i < argc; i++) {
    struct stat st; int r = stat(argv[i], &st);
    printf("PATH %s: %s\n", argv[i], r == 0 ? "VISIBLE" : en(errno));
  }
  return 0;
}

static int cmd_write(int argc, char **argv) {
  for (int i = 2; i < argc; i++) {
    int fd = open(argv[i], O_WRONLY | O_CREAT | O_TRUNC, 0600);
    if (fd < 0) printf("WRITE %s: %s\n", argv[i], en(errno));
    else { ssize_t r = write(fd, "x", 1); printf("WRITE %s: OK (wrote %zd)\n", argv[i], r); close(fd); }
  }
  return 0;
}

static void try_connect(const char *label, int fam, int type, const void *addr, socklen_t alen) {
  long t0 = ms_now();
  int s = socket(fam, type | SOCK_NONBLOCK, 0);
  if (s < 0) { printf("NET %s: socket(): %s\n", label, en(errno)); return; }
  int r = connect(s, (const struct sockaddr *)addr, alen);
  int e = errno;
  if (r == 0) { printf("NET %s: CONNECTED (reachable!) %ldms\n", label, ms_now() - t0); close(s); return; }
  if (e == EINPROGRESS) {
    struct pollfd p = {s, POLLOUT, 0};
    int pr = poll(&p, 1, 3000);
    int soerr = 0; socklen_t sl = sizeof soerr; getsockopt(s, SOL_SOCKET, SO_ERROR, &soerr, &sl);
    if (pr == 0) printf("NET %s: TIMEOUT after 3000ms\n", label);
    else if (soerr == 0) printf("NET %s: CONNECTED (reachable!) %ldms\n", label, ms_now() - t0);
    else printf("NET %s: %s after %ldms\n", label, en(soerr), ms_now() - t0);
  } else printf("NET %s: %s after %ldms\n", label, en(e), ms_now() - t0);
  close(s);
}

static int only_loopback(void) {
  FILE *f = fopen("/proc/net/dev", "r"); char l[256]; int ln = 0, ok = 1;
  if (!f) return 0;
  while (fgets(l, sizeof l, f)) if (++ln > 2) { char *c = strchr(l, ':'); if (c) { *c = 0; if (strcmp(l + strspn(l, " "), "lo")) ok = 0; } }
  fclose(f); return ok;
}

static int cmd_net(int argc, char **argv) {
  int ext = only_loopback();
  if (!ext) printf("NET external (192.0.2.1) probes SKIPPED: non-loopback interface visible, would not be safe\n");
  int port = argc > 2 ? atoi(argv[2]) : 0;
  struct sockaddr_in a4 = {.sin_family = AF_INET, .sin_port = htons(port)};
  inet_pton(AF_INET, "127.0.0.1", &a4.sin_addr);
  try_connect("tcp 127.0.0.1 (host fixture)", AF_INET, SOCK_STREAM, &a4, sizeof a4);
  struct sockaddr_in6 a6 = {.sin6_family = AF_INET6, .sin6_port = htons(argc > 3 ? atoi(argv[3]) : port)};
  inet_pton(AF_INET6, "::1", &a6.sin6_addr);
  try_connect("tcp [::1] (host fixture)", AF_INET6, SOCK_STREAM, &a6, sizeof a6);
  struct sockaddr_in d = {.sin_family = AF_INET, .sin_port = htons(80)};
  inet_pton(AF_INET, "192.0.2.1", &d.sin_addr);
  if (ext) try_connect("tcp 192.0.2.1:80 (RFC5737)", AF_INET, SOCK_STREAM, &d, sizeof d);
  /* UDP: connect+send; "sent" would mean a route exists */
  const char *tgt[] = {"192.0.2.1", "127.0.0.1"};
  for (int i = 0; i < 2; i++) {
    if (i == 0 && !ext) continue;
    long t0 = ms_now();
    int s = socket(AF_INET, SOCK_DGRAM, 0);
    struct sockaddr_in u = {.sin_family = AF_INET, .sin_port = htons(i == 0 ? 9 : (argc > 4 ? atoi(argv[4]) : port))};
    inet_pton(AF_INET, tgt[i], &u.sin_addr);
    ssize_t r = sendto(s, "x", 1, MSG_DONTWAIT, (struct sockaddr *)&u, sizeof u);
    int e = errno;
    printf("NET udp %s: sendto %s after %ldms\n", tgt[i], r == 1 ? (i == 0 ? "OK(sent, route exists!)" : "OK(into the sandbox-private loopback; host listener must stay at 0)") : en(e), ms_now() - t0);
    close(s);
  }
  FILE *f = fopen("/proc/net/dev", "r"); char l[256]; int ln = 0;
  printf("NETDEV:");
  while (f && fgets(l, sizeof l, f)) if (++ln > 2) { char *c = strchr(l, ':'); if (c) { *c = 0; printf(" %s", l + strspn(l, " ")); } }
  printf("\n"); if (f) fclose(f);
  f = fopen("/proc/net/route", "r"); ln = 0;
  while (f && fgets(l, sizeof l, f)) ln++;
  printf("ROUTE_LINES (1 = header only, i.e. no routes): %d\n", ln); if (f) fclose(f);
  return 0;
}

static void sc(const char *name, long r, int e) {
  printf("SYS %-22s ret=%ld errno=%s\n", name, r, r == -1 ? en(e) : "-");
}
#define T(name, expr) do { errno = 0; long _r = (expr); sc(name, _r, errno); } while (0)

static int cmd_sys(int argc, char **argv) {
  const char *mnt = argc > 2 ? argv[2] : "/job/mnt";
  /* ptrace: in a forked child so a successful TRACEME cannot disturb us */
  pid_t c = fork();
  if (c == 0) { errno = 0; long r = ptrace(PTRACE_TRACEME, 0, 0, 0); sc("ptrace(TRACEME)", r, errno); fflush(stdout); _exit(0); }
  int st; waitpid(c, &st, 0);
  T("bpf(BPF_MAP_CREATE)", syscall(SYS_bpf, 0, (void *)0, 0));
  T("io_uring_setup", syscall(SYS_io_uring_setup, 4, (void *)0));
  T("io_uring_enter", syscall(SYS_io_uring_enter, -1, 0, 0, 0, (void *)0, 0));
  T("io_uring_register", syscall(SYS_io_uring_register, -1, 0, (void *)0, 0));
  T("keyctl(GET_KEYRING_ID)", syscall(SYS_keyctl, 0, -2, 0));
  T("add_key", syscall(SYS_add_key, "user", "ms", "x", 1, -2));
  T("request_key", syscall(SYS_request_key, "user", "ms", (void *)0, 0));
  { struct { uint32_t type, size; uint64_t cfg; } attr = {1 /*SOFTWARE*/, 128, 0}; /* CPU_CLOCK-ish */
    T("perf_event_open", syscall(SYS_perf_event_open, &attr, 0, -1, -1, 0)); }
  T("mount(tmpfs)", syscall(SYS_mount, "none", mnt, "tmpfs", 0, (void *)0));
  T("umount2", syscall(SYS_umount2, mnt, 0));
  T("fsopen", syscall(SYS_fsopen, "tmpfs", 0));
  T("open_tree", syscall(SYS_open_tree, -1, "/", 0));
  T("move_mount", syscall(SYS_move_mount, -1, "", -1, "", 0));
  T("mount_setattr", syscall(SYS_mount_setattr, -1, "", 0, (void *)0, 0));
  T("pivot_root", syscall(SYS_pivot_root, "/", "/"));
  /* user namespace creation, in a child so success does not alter us */
  c = fork();
  if (c == 0) { errno = 0; long r = unshare(CLONE_NEWUSER); sc("unshare(CLONE_NEWUSER)", r, errno); fflush(stdout); _exit(0); }
  waitpid(c, &st, 0);
  fflush(stdout);
  c = fork();
  if (c == 0) { errno = 0; long r = syscall(SYS_clone, CLONE_NEWUSER | SIGCHLD, 0, 0, 0, 0); if (r == 0) _exit(0); sc("clone(CLONE_NEWUSER)", r, errno); fflush(stdout); _exit(0); }
  waitpid(c, &st, 0);
  fflush(stdout);
  c = fork();
  if (c == 0) { errno = 0; long r = unshare(0x40000000 /*CLONE_NEWNET*/); sc("unshare(CLONE_NEWNET)", r, errno); fflush(stdout); _exit(0); }
  waitpid(c, &st, 0);
  T("clone3(size=0)", syscall(SYS_clone3, (void *)0, 0));
  T("setns", syscall(SYS_setns, -1, 0));
  T("process_vm_readv", syscall(SYS_process_vm_readv, 1, (void *)0, 0, (void *)0, 0, 0));
  T("userfaultfd", syscall(SYS_userfaultfd, 0));
  T("kexec_load", syscall(SYS_kexec_load, 0, 0, (void *)0, 0));
  T("init_module", syscall(SYS_init_module, (void *)0, 0, ""));
  /* controls: must keep working */
  T("CONTROL getpid", syscall(SYS_getpid));
  T("CONTROL openat(/proc/self)", open("/proc/self/status", O_RDONLY));
  T("CONTROL clock_gettime", (long)time(NULL) > 0 ? 0 : -1);
  { int sp[2]; T("CONTROL socketpair(unix)", socketpair(AF_UNIX, SOCK_STREAM, 0, sp)); }
  { c = fork(); if (c == 0) _exit(0); waitpid(c, &st, 0); T("CONTROL fork+wait", c > 0 ? 0 : -1); }
  return 0;
}

static int cmd_priv(void) {
  T("setuid(0)", setuid(0));
  T("setresuid(0,0,0)", setresuid(0, 0, 0));
  T("setgid(0)", setgid(0));
  { struct __user_cap_header_struct h = {_LINUX_CAPABILITY_VERSION_3, 0};
    struct __user_cap_data_struct d[2] = {{0xffffffff, 0xffffffff, 0xffffffff}, {0xffffffff, 0xffffffff, 0xffffffff}};
    T("capset(all caps)", syscall(SYS_capset, &h, d)); }
  T("prctl(AMBIENT_RAISE,NET_ADMIN)", prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_RAISE, 12, 0, 0));
  T("prctl(NO_NEW_PRIVS get)", prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0));
  const char *w[] = {"/proc/sys/kernel/hostname", "/proc/sys/kernel/randomize_va_space", "/proc/sysrq-trigger", "/proc/sys/vm/drop_caches", "/proc/sys/net/ipv4/ip_forward"};
  for (unsigned i = 0; i < sizeof w / sizeof *w; i++) {
    errno = 0; int fd = open(w[i], O_WRONLY);
    printf("PROCWRITE %s: open(O_WRONLY) %s\n", w[i], fd >= 0 ? "OK(WRITABLE!)" : en(errno));
    if (fd >= 0) close(fd);
  }
  T("sethostname(x)", sethostname("x", 1));
  T("chown(/job,0,0)", chown("/job", 0, 0));
  return 0;
}

static int cmd_tiocsti(void) {
  char ch = 'Z';
  errno = 0; int r = ioctl(0, TIOCSTI, &ch);
  printf("TIOCSTI ioctl(0,TIOCSTI,inject): ret=%d errno=%s\n", r, r < 0 ? en(errno) : "-");
  errno = 0; int fd = open("/dev/tty", O_RDWR);
  printf("OPEN /dev/tty: %s\n", fd >= 0 ? "OK (controlling terminal reachable)" : en(errno));
  if (fd >= 0) {
    errno = 0; r = ioctl(fd, TIOCSTI, &ch);
    printf("TIOCSTI ioctl(/dev/tty,TIOCSTI,inject): ret=%d errno=%s\n", r, r < 0 ? en(errno) : "-");
    const char *m = "[probe wrote directly to controlling terminal via /dev/tty]\r\n";
    ssize_t w = write(fd, m, strlen(m)); printf("WRITE /dev/tty: %zd\n", w);
    close(fd);
  }
  printf("SID %d  PGRP %d  PID %d  tcgetsid(0)=%d\n", getsid(0), getpgrp(), getpid(), tcgetsid(0));
  return 0;
}

static int cmd_fds(void) {
  DIR *d = opendir("/proc/self/fd");
  int dfd = dirfd(d);
  struct dirent *de;
  int open_above2 = 0;
  while ((de = readdir(d))) {
    if (!is_num(de->d_name)) continue;
    int fd = atoi(de->d_name);
    if (fd == dfd) continue;
    char p[64], t[256]; snprintf(p, sizeof p, "/proc/self/fd/%d", fd);
    ssize_t n = readlink(p, t, sizeof t - 1); if (n < 0) n = 0; t[n] = 0;
    printf("FD %d -> %s\n", fd, t);
    if (fd > 2) open_above2++;
  }
  printf("FDS_ABOVE_2 %d\n", open_above2);
  closedir(d);
  return 0;
}

static int cmd_memhog(int argc, char **argv) {
  int mb = argc > 2 ? atoi(argv[2]) : 256;
  for (int i = 1; i <= mb; i++) {
    void *p = mmap(NULL, 1 << 20, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    if (p == MAP_FAILED) { printf("MEMHOG mmap failed at %d MB: %s\n", i, en(errno)); fflush(stdout); return 3; }
    memset(p, 0xA5, 1 << 20);
    if (i % 16 == 0) { printf("MEMHOG touched %d MB\n", i); fflush(stdout); }
  }
  printf("MEMHOG completed %d MB (NOT limited)\n", mb);
  return 0;
}

static int cmd_spawn(int argc, char **argv) {
  int want = argc > 2 ? atoi(argv[2]) : 100, ok = 0, err = 0;
  pid_t kids[512]; if (want > 512) want = 512;
  for (int i = 0; i < want; i++) {
    pid_t p = fork();
    if (p == 0) { sleep(4); _exit(0); }
    if (p < 0) { err = errno; break; }
    kids[ok++] = p;
  }
  printf("SPAWN requested=%d created=%d first_error=%s\n", want, ok, err ? en(err) : "none");
  fflush(stdout);
  for (int i = 0; i < ok; i++) kill(kids[i], SIGKILL);
  for (int i = 0; i < ok; i++) waitpid(kids[i], NULL, 0);
  return 0;
}

static int cmd_cpuspin(int argc, char **argv) {
  int secs = argc > 2 ? atoi(argv[2]) : 8;
  long t0 = ms_now(); volatile unsigned long x = 0;
  while (ms_now() - t0 < secs * 1000L) for (int i = 0; i < 1000000; i++) x += i;
  printf("CPUSPIN survived %d s wall (NOT limited)\n", secs);
  return 0;
}

static int cmd_fsize(int argc, char **argv) {
  int mb = argc > 2 ? atoi(argv[2]) : 8;
  signal(SIGXFSZ, SIG_IGN);
  int fd = open("fsize.out", O_WRONLY | O_CREAT | O_TRUNC, 0600);
  if (fd < 0) { printf("FSIZE open: %s\n", en(errno)); return 1; }
  char buf[1 << 20]; memset(buf, 'f', sizeof buf);
  long total = 0;
  int nfiles = argc > 3 ? atoi(argv[3]) : 1;
  if (nfiles > 1) {
    for (int k = 0; k < nfiles; k++) {
      char nm[64]; snprintf(nm, sizeof nm, "many-%d.out", k);
      int f2 = open(nm, O_WRONLY | O_CREAT | O_TRUNC, 0600); if (f2 < 0) { printf("FSIZE open %s: %s\n", nm, en(errno)); return 0; }
      for (int i = 0; i < mb; i++) { ssize_t w = write(f2, buf, sizeof buf); if (w != (ssize_t)sizeof buf) break; total += w; }
      close(f2);
    }
    printf("FSIZE_MANY wrote %ld bytes in %d files of <=%d MB each\n", total, nfiles, mb);
    return 0;
  }
  for (int i = 0; i < mb; i++) {
    ssize_t w = write(fd, buf, sizeof buf);
    if (w < 0) { printf("FSIZE write failed after %ld bytes: %s\n", total, en(errno)); return 0; }
    total += w; if (w != (ssize_t)sizeof buf) { printf("FSIZE short write after %ld bytes\n", total); return 0; }
  }
  printf("FSIZE wrote %ld bytes (NOT limited)\n", total);
  return 0;
}

static int cmd_flood(int argc, char **argv) {
  long mb = argc > 2 ? atol(argv[2]) : 8;
  char buf[1 << 16]; memset(buf, 'o', sizeof buf); buf[sizeof buf - 1] = '\n';
  for (long i = 0; i < mb * 16; i++) { if (write(1, buf, sizeof buf) < 0) return 1; }
  return 0;
}

static int cmd_abstract(int argc, char **argv) {
  const char *name = argc > 2 ? argv[2] : "ms-spike-abs";
  int s = socket(AF_UNIX, SOCK_STREAM, 0);
  struct sockaddr_un a = {.sun_family = AF_UNIX};
  size_t n = strlen(name); memcpy(a.sun_path + 1, name, n);
  errno = 0;
  int r = connect(s, (struct sockaddr *)&a, (socklen_t)(offsetof(struct sockaddr_un, sun_path) + 1 + n));
  printf("ABSTRACT connect(@%s): %s\n", name, r == 0 ? "CONNECTED (reachable!)" : en(errno));
  close(s);
  return 0;
}

static int cmd_shm(int argc, char **argv) {
  const char *name = argc > 2 ? argv[2] : "/ms-spike-shm";
  errno = 0; int fd = shm_open(name, O_RDWR, 0);
  printf("SHM shm_open(%s): %s\n", name, fd >= 0 ? "OPENED (reachable!)" : en(errno));
  if (fd >= 0) close(fd);
  DIR *d = opendir("/dev/shm");
  if (!d) { printf("SHM /dev/shm listing: %s\n", en(errno)); return 0; }
  struct dirent *de; int n = 0;
  while ((de = readdir(d))) if (de->d_name[0] != '.') { printf("SHM /dev/shm entry: %s\n", de->d_name); n++; }
  printf("SHM /dev/shm entries: %d\n", n);
  closedir(d);
  return 0;
}

static int cmd_ptrace(int argc, char **argv) {
  int pid = argc > 2 ? atoi(argv[2]) : 1;
  errno = 0; long r = ptrace(PTRACE_ATTACH, pid, 0, 0);
  printf("PTRACE_ATTACH %d: ret=%ld errno=%s\n", pid, r, r < 0 ? en(errno) : "-");
  if (r == 0) { waitpid(pid, NULL, 0); ptrace(PTRACE_DETACH, pid, 0, 0); }
  errno = 0; r = ptrace(PTRACE_SEIZE, pid, 0, 0);
  printf("PTRACE_SEIZE  %d: ret=%ld errno=%s\n", pid, r, r < 0 ? en(errno) : "-");
  if (r == 0) ptrace(PTRACE_DETACH, pid, 0, 0);
  return 0;
}

static int cmd_sha(void) {
  char h[65];
  if (sha_file("/proc/self/exe", h)) { printf("SHA error %s\n", en(errno)); return 1; }
  printf("SHA256_SELF %s\n%s\n", h, probe_id);
  return 0;
}

/* ---------------------------------------------------------------- landlock */
static int ll_abi(void) {
  return (int)syscall(SYS_landlock_create_ruleset, NULL, 0, LANDLOCK_CREATE_RULESET_VERSION);
}

static int cmd_ll_abi(void) {
  errno = 0; int v = ll_abi();
  if (v < 0) printf("LANDLOCK ABI: unavailable (%s)\n", en(errno)); else printf("LANDLOCK ABI: %d\n", v);
  return 0;
}

/* ll-write DIR [abstract-socket-name] : restrict fs writes to DIR (+ scoping if ABI allows) */
static int cmd_ll_write(int argc, char **argv) {
  const char *allow = argv[2];
  const char *abs = argc > 3 ? argv[3] : NULL;
  int abi = ll_abi();
  printf("LL abi=%d\n", abi);
  if (abi < 1) { printf("LL unavailable\n"); return 1; }
  uint64_t fs_all = LANDLOCK_ACCESS_FS_EXECUTE | LANDLOCK_ACCESS_FS_WRITE_FILE | LANDLOCK_ACCESS_FS_READ_FILE |
                    LANDLOCK_ACCESS_FS_READ_DIR | LANDLOCK_ACCESS_FS_REMOVE_DIR | LANDLOCK_ACCESS_FS_REMOVE_FILE |
                    LANDLOCK_ACCESS_FS_MAKE_CHAR | LANDLOCK_ACCESS_FS_MAKE_DIR | LANDLOCK_ACCESS_FS_MAKE_REG |
                    LANDLOCK_ACCESS_FS_MAKE_SOCK | LANDLOCK_ACCESS_FS_MAKE_FIFO | LANDLOCK_ACCESS_FS_MAKE_BLOCK |
                    LANDLOCK_ACCESS_FS_MAKE_SYM;
  uint64_t fs_write_only = fs_all & ~(LANDLOCK_ACCESS_FS_EXECUTE | LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR);
  if (abi >= 2) fs_write_only |= LANDLOCK_ACCESS_FS_REFER;
  if (abi >= 3) fs_write_only |= LANDLOCK_ACCESS_FS_TRUNCATE;
  struct landlock_ruleset_attr ra; memset(&ra, 0, sizeof ra);
  ra.handled_access_fs = fs_write_only;
#ifdef LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET
  if (abi >= 6) ra.scoped = LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | LANDLOCK_SCOPE_SIGNAL;
  printf("LL scoping requested: %s\n", abi >= 6 ? "abstract-unix-socket + signal" : "not available (ABI < 6)");
#else
  printf("LL scoping: headers lack LANDLOCK_SCOPE_* (ABI<6 headers)\n");
#endif
  size_t attr_size = sizeof ra;
  int rs = (int)syscall(SYS_landlock_create_ruleset, &ra, attr_size, 0);
  if (rs < 0) { printf("LL create_ruleset: %s\n", en(errno)); return 1; }
  int dfd = open(allow, O_PATH | O_DIRECTORY);
  struct landlock_path_beneath_attr pb = {.allowed_access = fs_write_only, .parent_fd = dfd};
  if (syscall(SYS_landlock_add_rule, rs, LANDLOCK_RULE_PATH_BENEATH, &pb, 0)) { printf("LL add_rule: %s\n", en(errno)); return 1; }
  if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)) { printf("LL nnp: %s\n", en(errno)); return 1; }
  if (syscall(SYS_landlock_restrict_self, rs, 0)) { printf("LL restrict_self: %s\n", en(errno)); return 1; }
  printf("LL ruleset applied (writes allowed only under %s)\n", allow);
  /* tests */
  char p[512]; int fd;
  snprintf(p, sizeof p, "%s/inside.txt", allow);
  fd = open(p, O_WRONLY | O_CREAT | O_TRUNC, 0600); printf("LL write inside  %s: %s\n", p, fd >= 0 ? "OK" : en(errno)); if (fd >= 0) close(fd);
  const char *outside = getenv("LL_OUTSIDE") ? getenv("LL_OUTSIDE") : "/tmp/ms-spike-sandbox-ll-outside.txt";
  fd = open(outside, O_WRONLY | O_CREAT | O_TRUNC, 0600); printf("LL write outside %s: %s\n", outside, fd >= 0 ? "OK(NOT restricted!)" : en(errno)); if (fd >= 0) { close(fd); unlink(outside); }
  fd = open("/proc/self/status", O_RDONLY); printf("LL read /proc/self/status (reads not handled): %s\n", fd >= 0 ? "OK" : en(errno)); if (fd >= 0) close(fd);
  if (abs) { int s = socket(AF_UNIX, SOCK_STREAM, 0); struct sockaddr_un a = {.sun_family = AF_UNIX}; size_t n = strlen(abs); memcpy(a.sun_path + 1, abs, n);
    errno = 0; int r = connect(s, (struct sockaddr *)&a, (socklen_t)(offsetof(struct sockaddr_un, sun_path) + 1 + n));
    printf("LL abstract connect(@%s) from host-side listener: %s\n", abs, r == 0 ? "CONNECTED (not scoped)" : en(errno)); close(s); }
  if (argc > 4) { int pid = atoi(argv[4]); errno = 0; int r = kill(pid, 0); printf("LL kill(%d,0) to unconfined same-uid process: %s\n", pid, r == 0 ? "OK (signal NOT scoped)" : en(errno)); }
  return 0;
}

/* ---------------------------------------------------------------- selftest */
static int st_fail;
static void chk(const char *name, int ok, const char *fmt, ...) {
  char d[256]; va_list ap; va_start(ap, fmt); vsnprintf(d, sizeof d, fmt, ap); va_end(ap);
  printf("CHECK %-14s %s %s\n", name, ok ? "PASS" : "FAIL", d);
  if (!ok) st_fail = 1;
}
static int status_val(const char *key, char *out, size_t n) {
  FILE *f = fopen("/proc/self/status", "r"); char l[256]; size_t kl = strlen(key);
  if (!f) return -1;
  while (fgets(l, sizeof l, f)) if (!strncmp(l, key, kl) && l[kl] == ':') { char *v = l + kl + 1; v += strspn(v, " \t"); v[strcspn(v, "\n")] = 0; snprintf(out, n, "%s", v); fclose(f); return 0; }
  fclose(f); return -1;
}
static int all_zero_hex(const char *s) { for (; *s; s++) if (*s != '0') return 0; return 1; }

/* selftest CANARY_NAME HOST_PIDNS_LINK */
static int cmd_selftest(int argc, char **argv) {
  extern char **environ;
  const char *canary = argc > 2 ? argv[2] : "MS_SELFTEST_CANARY";
  const char *hostpidns = argc > 3 ? argv[3] : "";
  char v[128];
  const char *caps[] = {"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"};
  int capok = 1; char capd[200] = "";
  for (int i = 0; i < 5; i++) { if (status_val(caps[i], v, sizeof v) || !all_zero_hex(v)) capok = 0; size_t l = strlen(capd); snprintf(capd + l, sizeof capd - l, "%s=%s ", caps[i], v); }
  chk("caps", capok, "%s", capd);
  status_val("NoNewPrivs", v, sizeof v); chk("no_new_privs", !strcmp(v, "1"), "NoNewPrivs=%s", v);
  status_val("Seccomp", v, sizeof v); chk("seccomp", !strcmp(v, "2"), "Seccomp=%s", v);
  { /* fds */ DIR *d = opendir("/proc/self/fd"); int dfd = dirfd(d), n = 0; struct dirent *de; char lst[128] = "";
    while ((de = readdir(d))) if (is_num(de->d_name)) { int fd = atoi(de->d_name); if (fd > 2 && fd != dfd) { n++; size_t l = strlen(lst); snprintf(lst + l, sizeof lst - l, "%d ", fd); } }
    closedir(d); chk("fds", n == 0, "open_fds_above_2=%d %s", n, lst); }
  { /* env */ int hit = 0, pids = 0; char names[256] = "";
    for (char **e = environ; *e; e++) { char *eq = strchr(*e, '='); size_t l = strlen(names); snprintf(names + l, sizeof names - l, "%.*s,", (int)(eq ? eq - *e : 0), *e); if (strstr(*e, canary)) hit++; }
    DIR *d = opendir("/proc"); struct dirent *de;
    while ((de = readdir(d))) if (is_num(de->d_name)) { pids++; char p[300]; snprintf(p, sizeof p, "/proc/%s/environ", de->d_name); int fd = open(p, O_RDONLY);
      if (fd >= 0) { char b[16384]; ssize_t r; while ((r = read(fd, b, sizeof b - 1)) > 0) { b[r] = 0; for (ssize_t i = 0; i < r; i++) if (!b[i]) b[i] = ' '; if (strstr(b, canary)) hit++; } close(fd); } }
    closedir(d);
    printf("ENVNAMES %s\n", names);
    chk("env_canary", hit == 0, "canary_hits=%d", hit);
    chk("pid_ns", pids <= 4 && getpid() < 10, "visible_pids=%d self=%d", pids, getpid()); }
  { char b[128]; ssize_t n = readlink("/proc/self/ns/pid", b, sizeof b - 1); if (n < 0) n = 0; b[n] = 0;
    chk("fresh_proc", n > 0 && strcmp(b, hostpidns) != 0, "pidns=%s host=%s", b, hostpidns); }
  { struct stat st; int sys_absent = stat("/sys", &st) != 0, run_absent = stat("/run", &st) != 0, home_absent = stat("/home", &st) != 0;
    errno = 0; int fd = open("/rootwrite", O_WRONLY | O_CREAT, 0600); int rootro = fd < 0; if (fd >= 0) { close(fd); unlink("/rootwrite"); }
    fd = open("/job/.selftest", O_WRONLY | O_CREAT | O_TRUNC, 0600); int jobw = fd >= 0; if (fd >= 0) { close(fd); unlink("/job/.selftest"); }
    chk("filesystem", sys_absent && run_absent && home_absent && rootro && jobw, "sys_absent=%d run_absent=%d home_absent=%d root_ro=%d job_writable=%d", sys_absent, run_absent, home_absent, rootro, jobw); }
  { int ok = 1; char d[300] = ""; long r; int e;
#define DENY(name, expr) do { errno = 0; r = (expr); e = errno; int g = (r == -1 && e == EPERM); if (!g) ok = 0; size_t l = strlen(d); snprintf(d + l, sizeof d - l, "%s=%s ", name, r == -1 ? en(e) : "ALLOWED"); } while (0)
    DENY("bpf", syscall(SYS_bpf, 0, (void *)0, 0));
    DENY("io_uring", syscall(SYS_io_uring_setup, 4, (void *)0));
    DENY("keyctl", syscall(SYS_keyctl, 0, -2, 0));
    { struct { uint32_t t, s; uint64_t c; } a = {1, 128, 0}; DENY("perf", syscall(SYS_perf_event_open, &a, 0, -1, -1, 0)); }
    DENY("mount", syscall(SYS_mount, "none", "/job", "tmpfs", 0, (void *)0));
    DENY("ptrace", ptrace(PTRACE_ATTACH, 1, 0, 0));
    pid_t c = fork(); if (c == 0) { errno = 0; long q = unshare(CLONE_NEWUSER); _exit(q == -1 && errno == EPERM ? 0 : 1); } int stt; waitpid(c, &stt, 0);
    int un = WIFEXITED(stt) && WEXITSTATUS(stt) == 0; if (!un) ok = 0; { size_t l = strlen(d); snprintf(d + l, sizeof d - l, "unshare_userns=%s ", un ? "EPERM" : "ALLOWED"); }
    chk("syscall_filter", ok, "%s", d); }
  { FILE *f = fopen("/proc/net/dev", "r"); char l[256], names[128] = ""; int ln = 0, only_lo = 1;
    while (f && fgets(l, sizeof l, f)) if (++ln > 2) { char *c2 = strchr(l, ':'); if (c2) { *c2 = 0; char *nm = l + strspn(l, " "); strcat(names, nm); strcat(names, " "); if (strcmp(nm, "lo")) only_lo = 0; } }
    if (f) fclose(f);
    struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(80)}; inet_pton(AF_INET, "192.0.2.1", &a.sin_addr);
    long t0 = ms_now(); int r2 = -1, e2 = 0; if (only_lo) { int s = socket(AF_INET, SOCK_STREAM | SOCK_NONBLOCK, 0); r2 = connect(s, (struct sockaddr *)&a, sizeof a); e2 = errno; close(s); }
    chk("network", only_lo && r2 == -1 && e2 == ENETUNREACH && ms_now() - t0 < 500, "ifaces=%sconnect(192.0.2.1)=%s", names, en(e2)); }
  printf("SELFTEST_RESULT %s\n", st_fail ? "FAIL" : "PASS");
  return st_fail;
}

static int cmd_cat(int argc, char **argv) {
  for (int i = 2; i < argc; i++) {
    FILE *f = fopen(argv[i], "r");
    if (!f) { printf("CAT %s: %s\n", argv[i], en(errno)); continue; }
    char l[512]; int n = 0;
    while (fgets(l, sizeof l, f) && n++ < 40) printf("CAT %s: %s", argv[i], l);
    if (n == 0) printf("CAT %s: <empty>\n", argv[i]);
    fclose(f);
  }
  return 0;
}

static int cmd_exec(int argc, char **argv) {
  if (argc < 3) return 2;
  execv(argv[2], argv + 2);
  printf("EXEC %s: %s\n", argv[2], en(errno));
  return 127;
}

/* 32-bit (int 0x80) and x32 ABI entry: the filter must not let them bypass the x86_64 numbers */
static int cmd_compat(void) {
  pid_t c = fork();
  if (c == 0) {
    long r;
    __asm__ volatile("int $0x80" : "=a"(r) : "0"(20L) : "memory"); /* i386 getpid */
    printf("COMPAT int0x80 getpid returned %ld (pid=%d)\n", r, getpid()); fflush(stdout); _exit(0);
  }
  int st; waitpid(c, &st, 0);
  if (WIFSIGNALED(st)) printf("COMPAT int0x80: process killed by signal %d (%s)\n", WTERMSIG(st), WTERMSIG(st) == SIGSYS ? "SIGSYS" : "other");
  else printf("COMPAT int0x80: process exited %d\n", WEXITSTATUS(st));
  errno = 0; long r = syscall(0x40000000L + 39 /* x32 getpid */);
  printf("COMPAT x32 getpid: ret=%ld errno=%s\n", r, r == -1 ? en(errno) : "-");
  return 0;
}

int main(int argc, char **argv) {
  setvbuf(stdout, NULL, _IOLBF, 0);
  if (argc < 2) { fprintf(stderr, "usage: probe <cmd>\n"); return 2; }
  const char *c = argv[1];
  if (!strcmp(c, "status")) return cmd_status();
  if (!strcmp(c, "env")) return cmd_env(argc > 2 ? argv[2] : "MS_SPIKE_MARKER");
  if (!strcmp(c, "pids")) return cmd_pids(argc, argv);
  if (!strcmp(c, "peek")) return cmd_peek(argc, argv);
  if (!strcmp(c, "proc")) return cmd_proc();
  if (!strcmp(c, "paths")) return cmd_paths(argc, argv);
  if (!strcmp(c, "write")) return cmd_write(argc, argv);
  if (!strcmp(c, "net")) return cmd_net(argc, argv);
  if (!strcmp(c, "sys")) return cmd_sys(argc, argv);
  if (!strcmp(c, "priv")) return cmd_priv();
  if (!strcmp(c, "tiocsti")) return cmd_tiocsti();
  if (!strcmp(c, "fds")) return cmd_fds();
  if (!strcmp(c, "memhog")) return cmd_memhog(argc, argv);
  if (!strcmp(c, "spawn")) return cmd_spawn(argc, argv);
  if (!strcmp(c, "cpuspin")) return cmd_cpuspin(argc, argv);
  if (!strcmp(c, "fsize")) return cmd_fsize(argc, argv);
  if (!strcmp(c, "flood")) return cmd_flood(argc, argv);
  if (!strcmp(c, "abstract")) return cmd_abstract(argc, argv);
  if (!strcmp(c, "shm")) return cmd_shm(argc, argv);
  if (!strcmp(c, "ptrace")) return cmd_ptrace(argc, argv);
  if (!strcmp(c, "sha")) return cmd_sha();
  if (!strcmp(c, "ll-abi")) return cmd_ll_abi();
  if (!strcmp(c, "ll-write")) return cmd_ll_write(argc, argv);
  if (!strcmp(c, "cat")) return cmd_cat(argc, argv);
  if (!strcmp(c, "exec")) return cmd_exec(argc, argv);
  if (!strcmp(c, "compat")) return cmd_compat();
  if (!strcmp(c, "selftest")) return cmd_selftest(argc, argv);
  if (!strcmp(c, "sleep")) { sleep(argc > 2 ? atoi(argv[2]) : 1); return 0; }
  if (!strcmp(c, "true")) return 0;
  fprintf(stderr, "unknown cmd %s\n", c);
  return 2;
}
