#!/usr/bin/env python3
"""Design validator for the database and API design documents (docs/design).

Reads the markdown and the OpenAPI file, checks structure and cross-document consistency,
and prints a pass and fail count. It changes nothing and installs nothing.
The OpenAPI file is converted to JSON with Ruby's bundled YAML library when ruby exists.
Without ruby the OpenAPI checks are skipped and reported as skipped.
It does NOT validate the file against the official OpenAPI 3.1 JSON Schema.

Usage: python3 scripts/validate-design.py
"""
import json
import os
import re
import shutil
import subprocess
import sys

ROOT = subprocess.run(['git', 'rev-parse', '--show-toplevel'], capture_output=True, text=True, check=True).stdout.strip()
os.chdir(ROOT)
passed = failed = skipped = 0
cat = 'database'
by_cat = {}

def check(name, ok, detail=''):
    global passed, failed
    c = by_cat.setdefault(cat, [0, 0])
    if ok:
        passed += 1
        c[0] += 1
    else:
        failed += 1
        c[1] += 1
        print('FAIL [%s] %s %s' % (cat, name, str(detail)[:400]))

def git(*args):
    return subprocess.run(['git'] + list(args), capture_output=True, text=True, cwd=ROOT)

def read(p):
    return open(p, encoding='utf-8').read()

DB = 'docs/design/database/'
API = 'docs/design/api/'
dd, ci, el, eh, mp, dm, dl = [read(DB + f) for f in ('data-dictionary.md', 'constraints-and-indexes.md', 'erd-logical.md', 'erd-high-level.md', 'migration-plan.md', 'domain-model.md', 'data-lifecycle.md')]
PRD = read('docs/PRD.md')

# ------------------------------------------------------------ database: dictionary
tables = {}
cur = None
for line in dd.split('\n'):
    m = re.match(r'### `(\w+)`', line)
    if m:
        cur = m.group(1)
        check('table name unique: ' + cur, cur not in tables)
        tables[cur] = []
        continue
    if cur and line.startswith('| `'):
        cells = [c.strip() for c in line.strip()[1:-1].split(' | ')]
        if len(cells) == 8:
            tables[cur].append(cells)
check('dictionary has tables', len(tables) >= 50, len(tables))
col_names = {t: [c[0].strip('`') for c in cols] for t, cols in tables.items()}
SENS = {'Internal', 'Target data', 'Authentication-derived (hash)', 'Untrusted text', 'Evidence metadata', 'Audit', 'Personal (low)', 'Secret reference (never the secret)'}
fk_single = []
for t, cols in tables.items():
    check('%s has columns' % t, len(cols) > 0)
    check('%s has a primary key' % t, any(c[3].startswith('PK') or ', PK' in c[3] or c[3].startswith('FK') and 'PK' in c[3] for c in cols), t)
    for c in cols:
        n = c[0].strip('`')
        check('%s.%s has type and nullability' % (t, n), c[1].strip('`') != '' and c[2] in ('NULL', 'NOT NULL'))
        check('%s.%s has a known sensitivity class' % (t, n), c[7] in SENS, c[7])
        if n.endswith('_at'):
            check('%s.%s is timestamptz' % (t, n), c[1] == '`timestamptz`', c[1])
        if n.endswith('_id'):
            check('%s.%s is uuid' % (t, n), c[1] == '`uuid`', c[1])
        if re.search(r'(password|secret|token|api_key|private)', n) and not n.endswith(('_hash', '_at')) and n not in ('secret_service_key',):
            check('%s.%s sensitive-looking column is not a plain value' % (t, n), False, 'name suggests a secret')
        if re.search(r'(password|secret|token)', n) and (n.endswith('_hash') or n == 'phc_hash'):
            check('%s.%s secret-derived column is class A' % (t, n), c[7] == 'Authentication-derived (hash)', c[7])
        for m in re.finditer(r'FK (\w+)\.id', c[3]):
            fk_single.append((t, n, m.group(1)))
check('phc_hash is a hash class', any(c[7] == 'Authentication-derived (hash)' for c in tables['user_credentials']))
for t, n, p in fk_single:
    check('FK %s.%s -> %s resolves' % (t, n, p), p in tables and 'id' in col_names[p], '')
check('no column stores a plaintext secret type (no column called password, secret or api_key)', not any(re.fullmatch(r'(password|secret|api_key|token|private_key)', n) for ns in col_names.values() for n in ns))

# composite foreign keys in the constraints document
fk_comp = re.findall(r'Foreign key `[^`]+`: `\(([^)]*)\)` -> `(\w+) \(([^)]*)\)`', ci)
check('constraints document lists foreign keys', len(fk_comp) > 80, len(fk_comp))
cdoc_tables = re.findall(r'### `(\w+)`', ci)
check('every table appears once in constraints-and-indexes', sorted(cdoc_tables) == sorted(tables), set(tables) ^ set(cdoc_tables))
cur = None
for line in ci.split('\n'):
    m = re.match(r'### `(\w+)`', line)
    if m:
        cur = m.group(1)
        continue
    m = re.match(r'- Foreign key `[^`]+`: `\(([^)]*)\)` -> `(\w+) \(([^)]*)\)`', line)
    if m and cur:
        cl = [x.strip() for x in m.group(1).split(',')]
        pc = [x.strip() for x in m.group(3).split(',')]
        ok = all(c in col_names[cur] for c in cl) and m.group(2) in tables and all(c in col_names[m.group(2)] for c in pc)
        check('FK %s(%s) -> %s(%s) resolves' % (cur, ','.join(cl), m.group(2), ','.join(pc)), ok)
        if len(cl) > 1:
            parent_cons = ci.split('### `%s`' % m.group(2))[1].split('### `')[0]
            check('composite FK target %s(%s) has a UNIQUE constraint' % (m.group(2), ','.join(pc)), re.search(r'UNIQUE \(%s\)' % re.escape(', '.join(pc)), parent_cons) is not None or pc == ['program_id', 'id'] and 'UNIQUE (id)' in parent_cons, pc)
for cc in range(1, 15):
    check('concurrency constraint CC-%02d is specified' % cc, 'CC-%02d' % cc in ci)
check('approval use uniqueness is specified', 'UNIQUE (approval_id, job_id)' in ci)
check('single unconfirmed attempt index is specified', 'job_attempts_unconfirmed_uq' in ci)
check('every index states a query pattern', all(len(r.split('|')) >= 3 for r in re.findall(r'(?m)^\| `\w+_(?:idx|uq)` \|.*$', ci)))
check('state transition CHECKs exist', 'CHECK legal pairs' in ci and 'only a human verifies' in ci)

# ------------------------------------------------------------ migrations
mig_ids = re.findall(r'(?m)^### (\d{5}) `(\w+)`', mp)
check('migrations are ordered and unique', [m[0] for m in mig_ids] == sorted(set(m[0] for m in mig_ids)) and len(mig_ids) == 16, mig_ids)
created = {}
for blk in re.split(r'(?m)^### ', mp)[1:]:
    mid = blk[:5]
    m = re.search(r'Tables created: (.*)', blk)
    for t in re.findall(r'`(\w+)`', m.group(1) if m else ''):
        check('table %s created in exactly one migration' % t, t not in created)
        created[t] = mid
    d = re.search(r'Depends on: (.*)', blk)
    if d:
        for x in re.findall(r'\d{5}', d.group(1)):
            check('migration %s depends only on an earlier migration (%s)' % (mid, x), x < mid)
check('every table is created by a migration', set(created) == set(tables), set(created) ^ set(tables))
cross = re.findall(r'`(\w+)\(([^)]*)\)` -> `(\w+)`', mp)
for child, cols, parent in cross:
    pass
fk_pairs = set()
for line in ci.split('\n'):
    pass
pair_in_mig = {}
for blk in re.split(r'(?m)^### ', mp)[1:]:
    mid = blk[:5]
    m = re.search(r'Cross-migration foreign keys created here[^:]*: (.*)', blk)
    if m:
        for ch, par in re.findall(r'`(\w+)\([^)]*\)` -> `(\w+)`', m.group(1)):
            pair_in_mig[(ch, par)] = mid
check('grants matrix and roles are documented', 'ms_migrator' in mp and 'ms_retention' in mp and 'Grants matrix' in mp)

# ------------------------------------------------------------ ERDs
def mermaid_blocks(txt):
    return re.findall(r'```mermaid\n(erDiagram.*?)```', txt, re.S)
fkset = set()
for t, cols in tables.items():
    for c in cols:
        for m in re.finditer(r'FK (\w+)\.id', c[3]):
            fkset.add((t, m.group(1), c[0].strip('`')))
for child, cols_, parent in re.findall(r'FK[^\n]*', '') or []:
    pass
for m in re.finditer(r'- Foreign key `[^`]+`: `\(([^)]*)\)` -> `(\w+) \(', ci):
    pass
cur = None
for line in ci.split('\n'):
    mm = re.match(r'### `(\w+)`', line)
    if mm:
        cur = mm.group(1)
    mm = re.match(r'- Foreign key `[^`]+`: `\(([^)]*)\)` -> `(\w+) \(', line)
    if mm and cur:
        for c in [x.strip() for x in mm.group(1).split(',')]:
            fkset.add((cur, mm.group(2), c))
seen_entities = set()
nedges = 0
for name, doc in (('erd-logical', el), ('erd-high-level', eh)):
    blocks = mermaid_blocks(doc)
    check('%s has diagrams' % name, len(blocks) >= 1)
    for b in blocks:
        ents = set(re.findall(r'(?m)^    (\w+) \{', b))
        seen_entities |= ents
        check('%s diagram entities are tables' % name, ents <= set(tables), ents - set(tables))
        for parent, card, child, label in re.findall(r'(?m)^    (\w+) (\S+) (\w+) : "(\w+)"', b):
            nedges += 1
            check('ERD edge %s -> %s (%s) matches a foreign key' % (parent, child, label), (child, parent, label) in fkset, '')
            check('ERD edge entities declared (%s, %s)' % (parent, child), parent in ents and child in ents)
check('every table appears in a logical ERD', set(tables) <= seen_entities, set(tables) - seen_entities)
for ch, par, col in sorted(fkset):
    if created.get(ch) and created.get(par) and created[ch] != created[par]:
        check('cross-migration FK %s -> %s is placed in the later migration' % (ch, par), pair_in_mig.get((ch, par)) == max(created[ch], created[par]), (pair_in_mig.get((ch, par)), created[ch], created[par]))
check('ERDs have many relationships', nedges > 100, nedges)
check('high-level ERD exists with ownership and operational classes', 'Ownership' in eh and 'Operational' in eh)

# ------------------------------------------------------------ domain model coverage
for t in ('assessments', 'jobs', 'job_attempts', 'approvals', 'lab_registrations', 'target_validations', 'refused_exclusions', 'plan_items', 'findings', 'reports', 'evidence_status', 'sessions'):
    check('state machine documented for %s' % t, ('| %s' % t) in dm, '')
frs = re.findall(r'(?m)^#### (FR-[A-Z]+-\d+)', PRD)
rows = {m.group(1): m.group(0) for m in re.finditer(r'(?m)^\| (FR-[A-Z]+-\d+) .*$', dm)}
check('domain model maps every PRD functional requirement', set(frs) <= set(rows), set(frs) - set(rows))
for fr in frs:
    r = rows.get(fr, '')
    cells = [c.strip() for c in r.strip()[1:-1].split(' | ')] if r else []
    if len(cells) == 4:
        won = 'Won\'t' in cells[3]
        check('%s has endpoints, internal owner or is excluded' % fr, cells[1] != 'none' or cells[3] != '' or won, r[:100])
for fr in re.findall(r'FR-[A-Z]+-\d+', dm):
    pass
check('data lifecycle distinguishes soft deletion, archival and permanent deletion', all(x in dl for x in ('Soft state', 'Archival', 'Permanent deletion')))
check('no cascade deletion in the design', 'ON DELETE CASCADE' not in dd + ci or 'No `ON DELETE CASCADE`' in dd or 'no `ON DELETE CASCADE`' in dd.lower() or 'Nothing is deleted with `ON DELETE CASCADE`' in dd)
check('audit table is append-only by design', 'append-only' in dd and 'forbid' in mp.lower() + ci.lower() + dd.lower() or 'BEFORE UPDATE' in ci)

# ------------------------------------------------------------ API
cat = 'api-inventory'
inv = read(API + 'endpoint-inventory.md')
spec = read(API + 'endpoint-specification.md')
mapd = read(API + 'database-mapping.md')
err = read(API + 'error-model.md')
inv_live = inv.split('## Endpoints in OpenAPI')[1].split('## Deferred endpoints')[0]
inv_rows = re.findall(r'(?m)^\| ([A-Z]+-\d+) \| (GET|POST|PUT|PATCH|DELETE) \| `([^`]+)` \| (PUB|R|W|RA) \|', inv_live)
check('inventory has endpoints', len(inv_rows) > 100, len(inv_rows))
ids = [r[0] for r in inv_rows]
check('endpoint ids are unique', len(ids) == len(set(ids)))
check('method and path pairs are unique', len(set((r[1], r[2]) for r in inv_rows)) == len(inv_rows))
total = re.search(r'\*\*Total\*\* \| \*\*(\d+)\*\*', inv)
check('inventory total matches the table', total and int(total.group(1)) == len(inv_rows))
readme = read(API + 'README.md')
check('API README states the endpoint count', ('%d endpoints in OpenAPI' % len(inv_rows)) in readme)
spec_ids = re.findall(r'(?m)^#### ([A-Z]+-\d+) (GET|POST|PUT|PATCH|DELETE) `/api/v1([^`]+)`', spec)
check('specification covers every endpoint exactly once', [s[0] for s in spec_ids] == ids, set(ids) ^ set(s[0] for s in spec_ids))
check('specification paths equal inventory paths', [(s[1], s[2]) for s in spec_ids] == [(r[1], r[2]) for r in inv_rows])
PROPS = ['Purpose', 'Authentication', 'Authorization', 'Re-authentication', 'CSRF', 'Request headers', 'Path parameters', 'Query parameters', 'Request schema', 'Response schema', 'Success status', 'Error responses', 'Idempotency', 'Audit event', 'Rate limit', 'Security considerations', 'PRD requirements']
blocks = re.split(r'(?m)^#### ', spec)[1:]
for b in blocks:
    eid = b.split(' ')[0]
    check('%s specification has every property' % eid, all(('| %s |' % p) in b for p in PROPS), eid)
    method = b.split(' ')[1]
    cls = [r[3] for r in inv_rows if r[0] == eid][0]
    if method != 'GET':
        check('%s requires CSRF' % eid, '| CSRF | Yes' in b, eid)
    if cls in ('W', 'RA'):
        check('%s state-changing endpoint has an audit event' % eid, re.search(r'\| Audit event \| `[a-z_.]+`', b) is not None, eid)
    if cls == 'RA':
        check('%s declares a re-authentication purpose' % eid, re.search(r'\| Re-authentication \| Yes, purpose `\w+`', b) is not None, eid)
map_ids = re.findall(r'(?m)^\| ([A-Z]+-\d+)( \(deferred\))? \|', mapd)
check('database mapping covers every endpoint', set(ids) <= set(m[0] for m in map_ids), set(ids) - set(m[0] for m in map_ids))
for line in mapd.split('\n'):
    mm = re.match(r'\| ([A-Z]+-\d+)[^|]* \| ([^|]*) \| (\w+) \| ([^|]*) \|', line)
    if mm:
        for t in [x.strip() for x in mm.group(4).split(',') if x.strip() and x.strip() != 'none']:
            if t in tables or t in ('none',):
                continue
            if re.fullmatch(r'\w+', t):
                check('mapping table %s exists (%s)' % (t, mm.group(1)), t in tables)
        for fr in re.findall(r'(?:N?FR)-[A-Z]+-\d+', mm.group(2)):
            check('mapping PRD id %s exists' % fr, fr in PRD)
codes = set(re.findall(r'(?m)^\| `([a-z_]+)` \| \d+ \|', err))
used = set(re.findall(r'\d+ `([a-z_]+)`', spec))
check('every error code used is in the catalog', used <= codes, used - codes)
statuses = {}
for c, s in re.findall(r'(?m)^\| `([a-z_]+)` \| (\d+) \|', err):
    statuses.setdefault(c, set()).add(s)
check('each error code has one HTTP status', all(len(v) == 1 for v in statuses.values()))
check('error model never exposes internals', all(x in err for x in ('Stack traces', 'SQL text', 'filesystem paths')))
dm_actions = set(re.findall(r'(?m)^\| `([a-z_.]+)` \|', dm.split('## 5. Audit event catalog')[1].split('## 6.')[0]))
for line in mapd.split('\n'):
    mm = re.match(r'\| ([A-Z]+-\d+)[^|]* \|.*\| `([a-z_.]+)` \|', line)
    if mm and not mm.group(1).startswith('WRK'):
        check('audit action %s of %s is in the domain-model catalog' % (mm.group(2), mm.group(1)), mm.group(2) in dm_actions)

# ------------------------------------------------------------ OpenAPI
cat = 'openapi-yaml-parse'
spec_path = API + 'openapi.yaml'
ruby = shutil.which('ruby')
doc = None
if ruby:
    r = subprocess.run([ruby, '-ryaml', '-rjson', '-e', 'puts JSON.generate(YAML.safe_load(File.read(ARGV[0]), aliases: false))', spec_path], capture_output=True, text=True)
    check('openapi.yaml parses as YAML', r.returncode == 0, r.stderr)
    if r.returncode == 0:
        doc = json.loads(r.stdout)
else:
    skipped += 1
    print('SKIPPED OpenAPI checks: ruby not found')
cat = 'openapi-structure'
if doc:
    check('openapi version is 3.1.x', str(doc.get('openapi', '')).startswith('3.1'))
    check('info has title and version', 'title' in doc['info'] and 'version' in doc['info'])
    check('servers declared', len(doc.get('servers', [])) >= 1)
    check('tags declared', len(doc.get('tags', [])) >= 10)
    ops = []
    opids = []
    for p, item in doc['paths'].items():
        check('path %s starts with /' % p, p.startswith('/'))
        for m, o in item.items():
            ops.append((o['x-endpoint-id'], m.upper(), p, o))
            opids.append(o['operationId'])
            check('%s has a 2xx response' % o['x-endpoint-id'], any(k.startswith('2') for k in o['responses']))
            check('%s has security declared' % o['x-endpoint-id'], 'security' in o)
            declared = {x['name'] for x in o.get('parameters', []) if 'name' in x} | {x['$ref'].split('Param_')[1] for x in o.get('parameters', []) if '$ref' in x and 'Param_' in x['$ref']}
            for pp in re.findall(r'\{(\w+)\}', p):
                check('%s declares path parameter %s' % (o['x-endpoint-id'], pp), pp in declared)
            if m.upper() != 'GET':
                sec = o.get('security', doc.get('security'))
                check('%s non-GET requires csrfToken' % o['x-endpoint-id'], any('csrfToken' in s for s in sec), sec)
            if o['x-route-class'] == 'PUB':
                check('%s public route has no session security' % o['x-endpoint-id'], not any('sessionCookie' in s for s in o.get('security', [])))
            else:
                check('%s authenticated route requires the session' % o['x-endpoint-id'], any('sessionCookie' in s for s in o.get('security', [])))
            if o['x-route-class'] in ('W', 'RA'):
                check('%s has x-audit-event' % o['x-endpoint-id'], o['x-audit-event'] != '-')
    check('operation ids are unique', len(opids) == len(set(opids)))
    check('OpenAPI endpoint ids equal the inventory', sorted(o[0] for o in ops) == sorted(ids), set(o[0] for o in ops) ^ set(ids))
    check('OpenAPI method and path equal the inventory', sorted((o[1], o[2]) for o in ops) == sorted((r[1], r[2]) for r in inv_rows))
    check('no deferred endpoint in OpenAPI', not any(o[0] == 'FND-13' for o in ops))
    # refs
    def walk(n, f):
        if isinstance(n, dict):
            for k, v in n.items():
                f(k, v)
                walk(v, f)
        elif isinstance(n, list):
            for v in n:
                walk(v, f)
    cat = 'openapi-refs'
    refs = []
    walk(doc, lambda k, v: refs.append(v) if k == '$ref' else None)
    bad = []
    for r in refs:
        parts = r.lstrip('#/').split('/')
        node = doc
        try:
            for p in parts:
                node = node[p]
        except (KeyError, TypeError):
            bad.append(r)
    check('every $ref resolves (%d refs)' % len(refs), not bad, bad[:5])
    cat = 'openapi-structure'
    schemes = doc['components']['securitySchemes']
    used_sec = set()
    walk(doc['paths'], lambda k, v: used_sec.update(s for item in v for s in item) if k == 'security' else None)
    check('every security scheme used is defined', used_sec <= set(schemes), used_sec - set(schemes))
    check('error responses reuse ErrorResponse', all('ErrorResponse' in json.dumps(v) for v in doc['components']['responses'].values()))
    # human-only invariant: no request schema carries an actor
    req_names = {o[3]['requestBody']['content']['application/json']['schema']['$ref'].split('/')[-1] for o in ops if 'requestBody' in o[3]}
    ACTOR = re.compile(r'^(actor|actorKind|actorUserId|approvedBy|verifiedBy|attestedBy|decidedBy|userId|createdBy|requestedBy)$')
    bad_fields = []
    for n in req_names:
        for f in doc['components']['schemas'][n].get('properties', {}):
            if ACTOR.match(f):
                bad_fields.append((n, f))
    check('no request schema can name an actor (human-only API)', not bad_fields, bad_fields)
    check('request schemas reject unknown fields', all(doc['components']['schemas'][n].get('additionalProperties') is False for n in req_names))
    # examples hygiene
    EX_BAD = [re.compile(p) for p in (r'AKIA[0-9A-Z]{16}', r'ghp_[A-Za-z0-9]{30,}', r'sk-[A-Za-z0-9]{32,}', r'-----BEGIN', r'/home/', r'://[^/\s:@]+:[^/\s@]+@', r'(?i)bearer\s+[a-z0-9._-]{20,}')]
    ex_text = json.dumps([s.get('example') for s in doc['components']['schemas'].values()])
    check('OpenAPI examples contain no secret-like or local-path text', not any(p.search(ex_text) for p in EX_BAD))
    hosts = set(re.findall(r'(?:https?://)?([a-z0-9.-]+\.(?:com|org|net|io|dev|sh))\b', ex_text))
    check('OpenAPI examples use only reserved example domains', not hosts, hosts)
    check('download endpoints are attachments', all('attachment' in o[3]['responses']['200']['description'] for o in ops if o[0] in ('EVD-05', 'RPT-07')))

# ------------------------------------------------------------ hardening checks
cat = 'cross-design'
import collections
dq = read('docs/design/dq-decisions.md')
dq_blocks = re.split(r'(?m)^## (DQ-\d+) ', dq)[1:]
dq_ids = dq_blocks[0::2]
check('DQ register has DQ-01 to DQ-18 in order', dq_ids == ['DQ-%02d' % i for i in range(1, 19)], dq_ids)
STAT = ('Decided', 'Proposed', 'Deferred', 'Owner Decision Required')
for i, body in zip(dq_ids, dq_blocks[1::2]):
    st = re.findall(r'\| Status \| ([^|]+) \|', body)
    check('%s has exactly one status from the four allowed' % i, len(st) == 1 and st[0].strip() in STAT, st)
    s = st[0].strip() if st else ''
    for f in ('Decision statement', 'Rationale', 'Security impact', 'Database impact', 'API impact', 'Related PRD requirements', 'Implementation milestone', 'Required verification', 'Residual risk'):
        check('%s has field %s' % (i, f), ('| %s |' % f) in body)
    if s == 'Decided':
        check('%s Decided names its authoritative source' % i, re.search(r'\| Source \| [^|]*\bD-\d+', body) is not None)
    if s == 'Deferred':
        check('%s Deferred has a trigger and a milestone' % i, 'Trigger' in body and re.search(r'Revisit', body) is not None)
    if s == 'Owner Decision Required':
        check('%s states the exact question' % i, 'Exact question' in body)
    if s == 'Proposed':
        check('%s Proposed is not described as approved' % i, not re.search(r'\| Basis \| [^|]*\b(approved by the owner|owner decided)', body, re.I))
    for fr in re.findall(r'(?:N?FR)-[A-Z]+-\d+', body):
        check('%s PRD id %s exists' % (i, fr), fr in PRD)
rec = read(DB + 'recovery-generation.md')
for h in ('Mechanism', 'Startup reconciliation', 'Reconciliation (AUTH-10)', 'Crash behavior', 'First-run initialization', 'Operator-visible state', 'Threat boundary and limitations', 'Required tests'):
    check('recovery-generation documents: ' + h, h in rec)
check('recovery-generation rejects a database-only detector', 'database-only boolean is rejected' in rec)
check('recovery-generation states the marker is not cryptographically protected', 'not cryptographically protected' in rec)
check('recovery_state table exists and is created in a migration', 'recovery_state' in tables and 'recovery_state' in re.search(r'### 00002.*?(?=### 00003)', mp, re.S).group(0))
check('no reusable re-authentication grant in the schema', 'reusable' not in col_names['reauth_grants'] and 'reauth_window_seconds' not in dd)
check('approvals have no non-human requester', 'requested_by_kind' not in col_names['approvals'] and 'requested_by_user_id' in col_names['approvals'])
check('claim recovery check CC-15 is specified', 'CC-15' in ci and 'recovery_state' in ci)
sc = read(DB + 'mvp-scope.md')
cls_rows = re.findall(r'(?m)^\| `(\w+)` \| (\w+) \| (MVP|SEC|LATER|DEF|CONS) \| ', sc)
check('every table is classified exactly once', sorted(r[0] for r in cls_rows) == sorted(tables), set(tables) ^ set(r[0] for r in cls_rows))
cnts = collections.Counter(r[2] for r in cls_rows)
tot = re.search(r'\| \*\*Total\*\* \| \| \*\*(\d+)\*\* \|', sc)
check('classification counts sum to the table count', tot and int(tot.group(1)) == len(tables) == sum(cnts.values()), cnts)
for k in ('MVP', 'SEC', 'LATER', 'DEF', 'CONS'):
    m = re.search(r'\| %s \| [^|]+ \| (\d+) \|' % k, sc)
    check('class count %s matches the rows' % k, m and int(m.group(1)) == cnts[k], cnts)
cl = read(API + 'endpoint-classification.md')
cl_rows = re.findall(r'(?m)^\| ([A-Z]+-\d+) \| (GET|POST|PUT|PATCH|DELETE) \| `([^`]+)` \| (MVP required|Security-critical|Later milestone|Deferred) \| (M\d+b?) \|', cl)
deferred_ids = set(re.findall(r'(?m)^\| ([A-Z]+-\d+) \| (?:GET|POST|PUT|PATCH|DELETE) \| `[^`]+` \| [^\n]*', inv.split('## Deferred endpoints')[1].split('## Internal')[0]))
check('classification covers every active and deferred endpoint once', sorted(r[0] for r in cl_rows) == sorted(set(ids) | deferred_ids), set(ids) ^ set(r[0] for r in cl_rows))
check('deferred endpoints are classified Deferred', all(r[3] == 'Deferred' for r in cl_rows if r[0] in deferred_ids))
check('only deferred endpoints are classified Deferred', all(r[0] in deferred_ids for r in cl_rows if r[3] == 'Deferred'))
check('classification keeps method and path of the inventory', all((r[1], r[2]) in {(x[1], x[2]) for x in inv_rows} for r in cl_rows if r[0] in ids))
if doc:
    cmap = {r[0]: (r[3], r[4]) for r in cl_rows}
    check('OpenAPI x-scope-class and x-milestone match the classification', all(cmap.get(o[0]) == (o[3]['x-scope-class'], o[3]['x-milestone']) for o in ops), '')
    check('no deferred endpoint is in OpenAPI', not any(o[0] in deferred_ids for o in ops))
    check('every RA operation is Security-critical', all(o[3]['x-scope-class'] == 'Security-critical' for o in ops if o[3]['x-route-class'] == 'RA'))
    res = [k for k in doc['components']['schemas']['SessionState']['properties']['state']['enum']]
    check('recovery_required is a documented session state', 'recovery_required' in res)
    check('recovery reconcile route exists and is public', any(o[0] == 'AUTH-10' and o[3]['x-route-class'] == 'PUB' for o in ops))
cat = 'public-docs'
check('no third-party license directory is tracked under docs', not os.path.exists('docs/development/third-party-licenses'))
gi = read('.gitignore')
check('.gitignore keeps local license copies out', '.kiro/skills-licenses/' in gi and '.kiro/skills/' in gi and not re.search(r'(?m)^\.kiro/?$', gi))
for must in ('.kiro/steering/security.md', '.kiro/specs/mvp/plan.md', '.kiro/hooks/skill-routing.json'):
    check('%s is not ignored' % must, git('check-ignore', '-q', '--no-index', '--', must).returncode != 0)
check('installed skill payload, generated Caveman rule and local license copies are ignored', all(git('check-ignore', '-q', '--no-index', '--', p).returncode == 0 for p in ('.kiro/skills/x/SKILL.md', '.kiro/skills-licenses/x.txt', '.kiro/skills-lock.json', '.kiro/steering/caveman.md')))
rd = read('README.md')
check('README references Apache-2.0 and limits it to project-authored material', 'Apache License 2.0' in rd and 'does not relicense third-party material' in rd)
import hashlib
check('root LICENSE is the canonical Apache-2.0 text', hashlib.sha256(open('LICENSE', 'rb').read()).hexdigest() == 'c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4')
inv_doc = read('docs/development/documentation-inventory.md')
tracked = [f for f in git('ls-files', 'docs', '.kiro', 'scripts', 'README.md', 'LICENSE', '.gitignore').stdout.split('\n') if f and os.path.exists(f)]
untracked = [f for f in git('ls-files', '-o', '--exclude-standard', 'docs', '.kiro', 'scripts').stdout.split('\n') if f]
entries = re.findall(r'(?m)^\| `([^`]+)` \|', inv_doc)
def covered(f):
    return any(f == e or (e.endswith('/') and f.startswith(e)) for e in entries)
missing = [f for f in sorted(set(tracked + untracked)) if not covered(f) and not f.startswith('scripts/spikes/')]
check('documentation inventory covers every documentation, config and script file', not missing, missing)
check('documentation inventory classifies with A, B, C or D', all(re.search(r'\| [ABCD] \|', l) for l in inv_doc.split('\n') if l.startswith('| `')))
check('docs/README.md index exists', os.path.exists('docs/README.md'))
check('design README files exist', os.path.exists(DB + 'README.md') and os.path.exists(API + 'README.md'))
# ------------------------------------------------------------ safety scan of the design folder
SECRET = [re.compile(p) for p in (r'AKIA[0-9A-Z]{16}', r'ghp_[A-Za-z0-9]{30,}', r'github_pat_[A-Za-z0-9_]{20,}', r'sk-[A-Za-z0-9]{32,}', r'-----BEGIN [A-Z ]*PRIVATE KEY-----', r'xox[baprs]-[A-Za-z0-9-]{10,}', r'(?<![A-Za-z0-9_])/home/(?!\.)[A-Za-z0-9._-]+/', r'(?<![A-Za-z0-9_])/Users/[A-Za-z0-9._-]+/')]
bad = []
for dirpath, _, files in os.walk('docs/design'):
    for f in files:
        txt = read(os.path.join(dirpath, f))
        if any(p.search(txt) for p in SECRET):
            bad.append(os.path.join(dirpath, f))
check('design documents contain no secret-like text or local paths', not bad, bad)
em = [os.path.join(dp, f) for dp, _, fs in os.walk('docs/design') for f in fs if '—' in read(os.path.join(dp, f))]
check('design documents contain no em dash', not em, em)
cat = 'public-docs'
gen = [os.path.join(dp, f) for root in ('services', 'apps') if os.path.isdir(root) for dp, dn, fs in os.walk(root) if 'node_modules' not in dp for f in fs if f.endswith('.gen.go') or 'openapi' in f.lower()]
# M3 (2026-11-18) owns the migrations, the queries and sqlc.yaml. Generated OpenAPI code still waits for G-openapi.
check('no OpenAPI-generated code exists before G-openapi', not gen, gen)
print()
print('%-22s %6s %6s' % ('category', 'pass', 'fail'))
for k, (p, f) in by_cat.items():
    print('%-22s %6d %6d' % (k, p, f))
print('%-22s %s' % ('openapi-official-schema', 'NOT VALIDATED AGAINST OFFICIAL OPENAPI 3.1 SCHEMA (not performed; no validator available, none installed)'))
print('TOTAL %d PASS %d FAIL %d SKIPPED %d' % (passed + failed, passed, failed, skipped))
sys.exit(1 if failed else 0)
