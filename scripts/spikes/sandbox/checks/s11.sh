#!/usr/bin/env bash
# S11: exact verified-byte execution (TOCTOU between hash and exec).
source "$(dirname "${BASH_SOURCE[0]}")/../lib.sh"; need_probes S11
ok=1; fail() { echo "  !! expectation failed: $1"; ok=0; }
w="$MS_SCRATCH/s11"; mkdir -p "$w"; chmod 700 "$w"
out=$(timeout 120 python3 "$FIX/toctou.py" "$BIN/probeA" "$BIN/probeB" "$w" 2>&1); echo "$out"
sum=$(echo "$out" | /usr/bin/grep '^SUMMARY')
# expected: (a) both NOT_GUARANTEED, (b) rename GUARANTEED / in-place NOT, (c) both GUARANTEED, c2 refused
has 'a_rename=NOT_GUARANTEED' "$sum" || fail "(a) rename swap should have succeeded"
has 'a_in-place=NOT_GUARANTEED' "$sum" || fail "(a) in-place swap should have succeeded"
has 'b_in-place=NOT_GUARANTEED' "$sum" || fail "(b) in-place overwrite of an open file should leak through"
has 'c_rename=GUARANTEED' "$sum" || fail "(c) rename"
has 'c_in-place=GUARANTEED' "$sum" || fail "(c) in-place"
has 'c2_refuse=GUARANTEED' "$sum" || fail "(c2) mismatch not refused"
if [ $ok = 1 ]; then verdict S11 PASS "path hash+exec is NOT safe (swap succeeded, both rename and in-place); plain open fd survives rename swap but NOT in-place overwrite; sealed memfd + bwrap --ro-bind-data --perms 0555 guarantees bytes run == bytes hashed under both swaps (works in bwrap 0.13.1). memfd + exec /proc/self/fd/N also works but leaves an fd open in the tool. Pinning covers only the tool binary, not libraries bound from the host"
else verdict S11 FAIL "see '!!' lines"; fi
