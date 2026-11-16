#!/usr/bin/env python3
"""Foundation validator for MORN1NGST4R.

Checks the decision register, cross-document references, the requirement and milestone
mapping, the threat model structure, the ignore rules and a repository safety scan.
It prints paths and categories only and never prints a matched secret value.
It changes nothing. Exit status is 1 when any check fails.

Usage: python3 scripts/validate-foundation.py
"""
import hashlib
import os
import re
import subprocess
import sys

ROOT = subprocess.run(['git', 'rev-parse', '--show-toplevel'], capture_output=True, text=True, check=True).stdout.strip()
os.chdir(ROOT)

passed = 0
failed = 0


def check(name, ok, detail=''):
    global passed, failed
    if ok:
        passed += 1
    else:
        failed += 1
        print('FAIL', name, detail[:300])


def read(path):
    with open(path, encoding='utf-8') as f:
        return f.read()


def git(*args):
    return subprocess.run(['git'] + list(args), capture_output=True, text=True, cwd=ROOT)


ARCH = read('.kiro/steering/architecture.md')
PLAN = read('.kiro/specs/mvp/plan.md')
PRD = read('docs/PRD.md')
TM = read('docs/security/threat-model.md')

# ---------------------------------------------------------------- 1. decision register
rows = [l for l in ARCH.split('\n') if l.startswith('| ')]
d_ids = [m.group(1) for l in rows for m in [re.match(r'\| (D-\d+) \|', l)] if m]
p_ids = [m.group(1) for l in rows for m in [re.match(r'\| (P-\d+) \|', l)] if m]
o_rows = [(m.group(1), l) for l in rows for m in [re.match(r'\| (O-\d+) \|', l)] if m]
o_ids = [o for o, _ in o_rows]
check('D ids unique', len(d_ids) == len(set(d_ids)), str([x for x in d_ids if d_ids.count(x) > 1]))
check('D ids contiguous 1..%d' % len(d_ids), sorted(int(x[2:]) for x in d_ids) == list(range(1, len(d_ids) + 1)))
check('D-91 is the last decision', max(int(x[2:]) for x in d_ids) == 91)
check('P ids unique', len(p_ids) == len(set(p_ids)), str([x for x in p_ids if p_ids.count(x) > 1]))
check('P ids cover 1..64', set(int(x[2:]) for x in p_ids) == set(range(1, 65)))
check('O ids unique', len(o_ids) == len(set(o_ids)), str([x for x in o_ids if o_ids.count(x) > 1]))
check('O ids cover 1..43', set(int(x[2:]) for x in o_ids) == set(range(1, 44)))
open_section = ARCH.split('### Open (not decided)')[1].split('### Resolved by the MVP decision')[0]
still_open = sorted(re.findall(r'(?m)^\| (O-\d+) \|', open_section))
check('open table holds exactly the eight gated items', still_open == ['O-07', 'O-13', 'O-22', 'O-32', 'O-33', 'O-35', 'O-36', 'O-38'], str(still_open))
for oid in still_open:
    line = [l for o, l in o_rows if o == oid][0]
    check('open item %s states its gate' % oid, 'Round 6:' in line)
resolved_section = ARCH.split('### Resolved by the MVP decision')[1].split('### Not adopted')[0]
check('every other O item is in the Resolved table', all(('| %s |' % ('O-%02d' % n)) in resolved_section for n in range(1, 44) if 'O-%02d' % n not in still_open))

# every reference to a register id resolves
DOCS = ['.kiro/steering/security.md', '.kiro/steering/product.md', '.kiro/steering/structure.md', '.kiro/steering/tech.md',
        '.kiro/steering/architecture.md', '.kiro/specs/mvp/plan.md', 'docs/PRD.md', 'docs/security/threat-model.md',
        'docs/decisions/README.md', 'docs/decisions/0001-owner-security-decisions-round-2.md',
        'docs/decisions/0002-foundation-completion-decisions.md', 'docs/decisions/0003-foundation-closure-decisions.md', 'docs/decisions/0004-health-endpoints-mvp-decision.md', 'docs/decisions/0005-m1-scope-engine-closure-decisions.md', 'README.md']
# Decision records after 0003 (D-86 onward, records 0004 and 0005) define their own identifiers. The register in architecture.md must list them too.
later_d = []
for f in sorted(os.listdir('docs/decisions')):
    if re.match(r'000[4-9]-.*\.md$', f) or re.match(r'00[1-9]\d-.*\.md$', f):
        later_d += re.findall(r'(?m)^\| (D-\d+) \|', read('docs/decisions/' + f))
check('decision records after 0003 continue the register without gaps', sorted(int(x[2:]) for x in later_d) == list(range(86, 86 + len(later_d))), str(later_d))
D_SET, P_SET, O_SET = set(d_ids) | set(later_d), set(p_ids), set(o_ids)
check('decisions defined in records after 0003 are in the architecture register', set(later_d) <= set(d_ids), str(sorted(set(later_d) - set(d_ids))))
for doc in DOCS:
    txt = read(doc)
    bad = set()
    for m in re.finditer(r'(?<![A-Za-z-])D-(\d{2})(?!\d)', txt):
        if 'D-%s' % m.group(1) not in D_SET:
            bad.add('D-' + m.group(1))
    for m in re.finditer(r'(?<![A-Za-z-])P-(\d{2})(?!\d)', txt):
        if 'P-%s' % m.group(1) not in P_SET:
            bad.add('P-' + m.group(1))
    for m in re.finditer(r'(?<![A-Za-z-])O-(\d{2})(?!\d)', txt):
        if 'O-%s' % m.group(1) not in O_SET:
            bad.add('O-' + m.group(1))
    check('register references resolve in ' + doc, not bad, str(sorted(bad)))

# 0003 table and register agree
rec = read('docs/decisions/0003-foundation-closure-decisions.md')
rec_ids = set(re.findall(r'(?m)^\| (D-\d+) \|', rec))
check('record 0003 lists D-63..D-85', rec_ids == set('D-%d' % n for n in range(63, 86)), str(sorted(rec_ids)))

# ---------------------------------------------------------------- 2. threat model
t_rows = []
for l in TM.split('\n'):
    m = re.match(r'\| (T-\d+) \|', l)
    if m and ' | ' in l:
        t_rows.append((m.group(1), [c.strip() for c in l.strip()[1:-1].split(' | ')]))
reg = [(i, c) for i, c in t_rows if len(c) == 12]
check('threat register has 73 rows', len(reg) == 73, str(len(reg)))
ids = [i for i, _ in reg]
check('threat ids unique and contiguous', ids == ['T-%02d' % n for n in range(1, 74)])
check('every threat has all 12 fields non-empty', all(all(x for x in c) for _, c in reg))
STATUS = {'Mitigated and tested', 'Feasibility shown', 'Specified', 'Partially specified', 'Unresolved', 'Accepted risk'}
check('threat statuses are from the vocabulary', all(c[7] in STATUS for _, c in reg))
check('no threat is "Mitigated and tested" (no control is implemented and tested end to end)', not any(c[7] == 'Mitigated and tested' for _, c in reg))
check('priorities are High, Medium or Low', all(c[10] in ('High', 'Medium', 'Low') for _, c in reg))
imp = re.findall(r'(?m)^\| (T-\d+) \| (.+) \| (High|Medium|Low) \|$', TM.split('### 7.4 Impact by threat')[1].split('## 8. Abuse cases')[0])
check('impact table covers all 73 threats', [i for i, _, _ in imp] == ids)
check('impact priority matches register', all(p == c[10] for (i, _, p), (_, c) in zip(imp, reg)))
cnt_line = re.search(r'The register has (\d+) rows', TM)
check('stated row count matches', cnt_line and int(cnt_line.group(1)) == 73)
tm_refs = set(int(x) for x in re.findall(r'(?<![A-Za-z-])T-(\d{2})(?!\d)', '\n'.join(read(d) for d in DOCS)))
check('threat references resolve', tm_refs <= set(range(1, 74)), str(sorted(tm_refs - set(range(1, 74)))))
total_line = re.search(r'\| Total \| (\d+) \| (\d+) \| (\d+) \| (\d+) \|', TM)
check('counts table total matches', total_line and int(total_line.group(4)) == 73)

# ---------------------------------------------------------------- 3. PRD
blocks = re.split(r'(?m)^(?=#### )', PRD)
frs = {}
for b in blocks:
    m = re.match(r'#### (FR-[A-Z]+-\d+) ', b)
    if m:
        frs[m.group(1)] = b
check('PRD has 48 functional requirements', len(frs) == 48, str(len(frs)))
FIELDS = ['Requirement', 'User value', 'Priority', 'Acceptance criteria', 'Dependencies', 'Security', 'MVP status', 'Source']
for fid, b in frs.items():
    check('%s has all eight fields' % fid, all(('- **%s:**' % f) in b for f in FIELDS))
check('PRD FR ids unique', len(re.findall(r'(?m)^#### FR-', PRD)) == len(frs))
nfr = re.findall(r'(?m)^\| (NFR-[A-Z]+-\d+) \|', PRD)
check('PRD has 25 NFRs, unique', len(nfr) == 25 and len(set(nfr)) == 25, str(len(nfr)))
check('no requirement is marked open', 'OPEN — OWNER DECISION REQUIRED' not in ''.join(frs.values()) and not re.search(r'\| `OPEN — OWNER', PRD))
check('no FR status claims implementation', not any(re.search(r'- \*\*MVP status:\*\* (Implemented|Done|Complete)', b) for b in frs.values()))
check('no FR is "Not in plan"', not any(re.search(r'- \*\*MVP status:\*\* Not in plan', b) for b in frs.values()))
all_fr_ref = set(re.findall(r'(?<!N)FR-[A-Z]+-\d{3}', '\n'.join([PRD, PLAN])))
check('FR references resolve', all_fr_ref <= set(frs), str(sorted(all_fr_ref - set(frs))))
all_nfr_ref = set(re.findall(r'NFR-[A-Z]+-\d{3}', '\n'.join([PRD, PLAN])))
check('NFR references resolve', all_nfr_ref <= set(nfr), str(sorted(all_nfr_ref - set(nfr))))
ux = set(re.findall(r'(?m)^\| (UX-\d+) \|', PRD))
sm = set(re.findall(r'(?m)^\| (SM-\d+) \|', PRD))
check('PRD has 11 UX and 9 SM items', len(ux) == 11 and len(sm) == 9)
check('UX and SM references resolve', set(re.findall(r'UX-\d{3}', PLAN)) <= ux and set(re.findall(r'SM-\d{2}', PLAN)) <= sm)

# ---------------------------------------------------------------- 4. plan milestones
mrows = {}
mtable = PLAN.split('## Milestone map')[1].split('Recommended build order')[0]
for l in mtable.split('\n'):
    m = re.match(r'\| (M\d+b?) \| ([^|]+) \| ([^|]+) \| ([^|]+) \| ([^|]+) \|$', l)
    if m:
        deps = re.findall(r'M\d+b?', m.group(3))
        # expand "M16 to M23"
        for a, b in re.findall(r'M(\d+) to M(\d+)', m.group(3)):
            deps += ['M%d' % n for n in range(int(a), int(b) + 1)]
        mrows[m.group(1)] = (deps, m.group(4))
EXPECT = ['M0', 'M1', 'M1b'] + ['M%d' % n for n in range(2, 26)] + ['M22b']
check('milestone map has every milestone', sorted(mrows) == sorted(EXPECT), str(sorted(set(EXPECT) ^ set(mrows))))
check('every dependency exists', all(d in mrows for deps, _ in mrows.values() for d in deps))
order = re.search(r'Recommended build order: ([^\n]+?)\. M1b runs', PLAN)
seq = [x.strip() for x in order.group(1).split(',')] if order else []
check('build order lists every milestone except M1b once', sorted(seq) == sorted(set(mrows) - {'M1b'}), str(sorted(set(mrows) - {'M1b'} ^ set(seq))))
pos = {m: i for i, m in enumerate(seq)}
bad = [(m, d) for m in seq for d in mrows[m][0] if d != 'M1b' and (d not in pos or pos[d] >= pos[m])]
check('every milestone is built after its dependencies', not bad, str(bad))
# acyclic
state = {}
def visit(n, stack):
    if state.get(n) == 2:
        return True
    if state.get(n) == 1:
        return False
    state[n] = 1
    for d in mrows[n][0]:
        if not visit(d, stack):
            return False
    state[n] = 2
    return True
check('milestone dependencies are acyclic', all(visit(n, []) for n in mrows))
defs = set(re.findall(r'(?m)^### (M\d+b?):', PLAN)) | {'M1'}
check('every milestone has a definition and acceptance', set(mrows) <= defs, str(sorted(set(mrows) - defs)))
for m in sorted(set(mrows) - {'M1'}):
    sec = re.search(r'(?ms)^### %s: .*?(?=^### |^## )' % re.escape(m), PLAN)
    check('%s definition states Acceptance and Depends' % m, bool(sec) and 'Acceptance' in sec.group(0) and ('Depends on' in sec.group(0) or m == 'M0'))
# requirement coverage
def expand(text):
    out = set(re.findall(r'FR-[A-Z]+-\d{3}', text))
    for pre, a, b in re.findall(r'(FR-[A-Z]+-)(\d{3}) to (?:FR-[A-Z]+-)?(\d{3})', text):
        out |= set('%s%03d' % (pre, n) for n in range(int(a), int(b) + 1))
    return out
covered = set()
for _, cov in mrows.values():
    covered |= expand(cov)
out_of_mvp = {f for f, b in frs.items() if re.search(r'- \*\*MVP status:\*\* Out of MVP', b)}
check('every FR is covered by a milestone or Out of MVP', set(frs) - covered - out_of_mvp == set(), str(sorted(set(frs) - covered - out_of_mvp)))
for must in ['FR-SCOPE-001', 'FR-SCOPE-002']:
    check('%s maps to M1' % must, must in expand(mrows['M1'][1]))
check('plan has no ticked task', '- [x]' not in PLAN)
prd_ms = set(re.findall(r'(?m)^\| (M\d+b?) \| ', PRD.split('## 3.16 Release Roadmap')[1].split('**Plan mismatches')[0]))
check('PRD roadmap lists the same milestones as the plan', prd_ms == set(mrows), str(sorted(prd_ms ^ set(mrows))))
check('M1 does not depend on tool execution', mrows['M1'][0] == ['M0'])

# ---------------------------------------------------------------- 5. text hygiene
NEW_DOCS = ['docs/PRD.md', 'docs/decisions/0003-foundation-closure-decisions.md', 'docs/decisions/0004-health-endpoints-mvp-decision.md', 'docs/decisions/0005-m1-scope-engine-closure-decisions.md', 'docs/decisions/README.md', 'README.md']
for d in NEW_DOCS:
    txt = read(d).replace('`OPEN — OWNER DECISION REQUIRED`', '')
    check('no em dash in ' + d, '—' not in txt)

# ---------------------------------------------------------------- 6. license
lic = open('LICENSE', 'rb').read()
check('LICENSE is the canonical Apache-2.0 text', hashlib.sha256(lic).hexdigest() == 'c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4' and len(lic) == 11357)

# ---------------------------------------------------------------- 7. ignore rules
MUST_IGNORE = '''.env .env.local .env.production prod.env .envrc .kiro/skills/foo/SKILL.md .kiro/skills-lock.json .kiro/settings/mcp.json
.superpowers/x .worktrees/a/b .taste-skill/x server.pem tls/private.key cert.p12 id_ed25519 id_rsa deploy.kdbx secrets/a credentials.json token.json
.netrc cookies.txt storage-state.json config.local.yaml db.local.json app.local data/pg/PG_VERSION data/evidence/a.bin pgdata/base postgres-data/x
.morningstar/x evidence/a.bin reports/r.md exports/e.csv output/o.json results/r scans/s recon/r workspaces/w jobs/j screenshots/a.png dump.sql.gz
backup.dump morningstar.sqlite3 local.db capture.har traffic.pcap traffic.pcapng out.nmap nuclei-output.jsonl httpx-output.txt nuclei-templates/x
app.log logs/server.log audit.log core.12345 crash.dmp x.tmp tmp/x postmaster.pid app.sock services/core/bin/server services/core/server.exe
services/core/coverage.out cpu.prof __debug_bin123 go.work go.work.sum svc.test node_modules/a/b apps/web/node_modules/a apps/web/.next/cache
apps/web/out/index.html apps/web/build/x apps/web/dist/x apps/web/next-env.d.ts apps/web/tsconfig.tsbuildinfo apps/web/playwright-report/index.html
test-results/x packages/api-client/dist/index.js apps/web/.eslintcache dist/x build/x bin/x coverage/x __pycache__/a.pyc a.pyc .venv/bin/python
.pytest_cache/x .DS_Store Thumbs.db a.swp b~ .idea/x .vscode/settings.json'''.split()
MUST_TRACK = '''.env.example .env.local.example .gitignore LICENSE README.md Makefile .kiro/steering/security.md .kiro/steering/tech.md .kiro/specs/mvp/plan.md
.kiro/hooks/skill-routing.json .kiro/agents/a.json docs/PRD.md docs/security/threat-model.md docs/decisions/0003-foundation-closure-decisions.md
scripts/sync-agent-skills.sh scripts/validate-foundation.py scripts/spikes/egress/go.mod
scripts/spikes/egress/last-run/e1.log scripts/spikes/sandbox/sandbox.py services/core/go.mod services/core/go.sum services/core/sqlc.yaml
services/core/internal/domain/scope/host.go services/core/internal/domain/evidence/evidence.go services/core/internal/domain/job/job.go
services/core/internal/domain/approval/approval.go services/core/internal/application/evidence/service.go
services/core/internal/adapters/outbound/evidence/store.go services/core/internal/adapters/outbound/credentials/secretservice.go
services/core/internal/adapters/outbound/tools/subfinder.go services/core/internal/adapters/outbound/persistence/queries.go
services/core/migrations/0001_init.sql services/core/queries/scope.sql services/core/testdata/fuzz/FuzzParseHost/seed1
services/core/tests/architecture/deps_test.go services/core/cmd/server/main.go services/core/cmd/worker/main.go apps/web/package.json
apps/web/package-lock.json apps/web/tsconfig.json apps/web/app/page.tsx apps/web/app/build/page.tsx apps/web/app/data/page.tsx
apps/web/lib/data/client.ts apps/web/components/ui/button.tsx apps/web/features/evidence/list.tsx apps/web/services/api.ts apps/web/next.config.ts
packages/api-client/package.json packages/api-client/src/index.ts tests/fixtures/sample.json tests/e2e/flow.spec.ts tests/security/scope_test.go
skills/security-assessment/SKILL.md skills/manifest/schema.json openapi/openapi.yaml id_rsa.pub'''.split()
def ignored(p):
    return git('check-ignore', '-q', '--no-index', '--', p).returncode == 0
bad_i = [p for p in MUST_IGNORE if not ignored(p)]
bad_t = [p for p in MUST_TRACK if ignored(p)]
check('%d representative paths are ignored' % len(MUST_IGNORE), not bad_i, str(bad_i))
check('%d representative source and template paths stay trackable' % len(MUST_TRACK), not bad_t, str(bad_t))
check('.gitignore is not itself ignored', not ignored('.gitignore'))
tracked_ignored = [f for f in git('ls-files', '-ci', '--exclude-standard').stdout.split() if os.path.exists(f)]  # a file deleted from the working tree but not yet committed is not an exposure
check('no tracked file is matched by an ignore rule', not tracked_ignored, str(tracked_ignored))
staged = git('diff', '--cached', '--name-only').stdout.split('\n')
staged = [s for s in staged if s]
bad_staged = [s for s in staged if ignored(s) and not s.startswith('scripts/spikes/')]
check('no staged file matches an ignore rule', not bad_staged, str(bad_staged))
check('no skill payload is staged or visible to Git', not any(s.startswith('.kiro/skills/') or s == '.kiro/skills-lock.json' for s in staged) and not git('ls-files', '-o', '--exclude-standard', '.kiro/skills').stdout.strip())

# ---------------------------------------------------------------- 8. safety scan (tracked + staged + untracked and not ignored)
files = set(git('ls-files').stdout.split('\n')) | set(git('ls-files', '-o', '--exclude-standard').stdout.split('\n'))
files = sorted(f for f in files if f and os.path.isfile(f) and not os.path.islink(f))
links = [f for f in set(git('ls-files').stdout.split('\n')) | set(git('ls-files', '-o', '--exclude-standard').stdout.split('\n')) if f and os.path.islink(f)]
check('no symlinks in the tracked or untracked tree', not links, str(links))
big = [f for f in files if os.path.getsize(f) > 1024 * 1024]
check('no file over 1 MB', not big, str(big))
SECRET = [re.compile(p) for p in [r'AKIA[0-9A-Z]{16}', r'ghp_[A-Za-z0-9]{30,}', r'github_pat_[A-Za-z0-9_]{20,}', r'sk-[A-Za-z0-9]{32,}',
          r'-----BEGIN [A-Z ]*PRIVATE KEY-----', r'xox[baprs]-[A-Za-z0-9-]{10,}', r'AIza[0-9A-Za-z_-]{35}', r'_authToken\s*=', r'(?i)bearer\s+[A-Za-z0-9._-]{24,}']]
PATHRE = re.compile(r'((?<![A-Za-z0-9_])/home/(?!\.)[A-Za-z0-9._-]+/|(?<![A-Za-z0-9_])/Users/[A-Za-z0-9._-]+/|C:\\Users\\)')
URLCRED = re.compile(r'://[^/\s:@]+:[^/\s@]+@')
URLCRED_FIXTURES = {'scripts/spikes/egress/cmd/spike/e4.go', 'scripts/spikes/egress/last-run/e4.log', 'scripts/spikes/egress/last-run/offline-e4.log',
                    'docs/security/feasibility/egress-spike.md'}
hits = {'secret': [], 'local path': [], 'url credentials': [], 'binary': []}
for f in files:
    raw = open(f, 'rb').read()
    if raw[:4] in (b'\x7fELF', b'MZ\x90\x00', b'\xcf\xfa\xed\xfe', b'\xca\xfe\xba\xbe'):
        hits['binary'].append(f)
        continue
    try:
        txt = raw.decode('utf-8')
    except UnicodeDecodeError:
        hits['binary'].append(f)
        continue
    if any(r.search(txt) for r in SECRET):
        hits['secret'].append(f)
    if PATHRE.search(txt):
        hits['local path'].append(f)
    if URLCRED.search(txt) and f not in URLCRED_FIXTURES:
        hits['url credentials'].append(f)
for k, v in hits.items():
    check('safety scan: no %s in %d files' % (k, len(files)), not v, str(v))

print('TOTAL %d PASS %d FAIL %d' % (passed + failed, passed, failed))
sys.exit(1 if failed else 0)
