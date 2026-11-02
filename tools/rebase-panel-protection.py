#!/usr/bin/env python3
"""Rebase reviewed TLS/VLESS panel protection conflicts; fail closed on drift."""
import re
import subprocess
import sys
from pathlib import Path

REVIEWED = {
    "common/tls/reality_client.go": 3,
    "option/tls.go": 1,
    "protocol/vless/outbound.go": 4,
}
MARKERS = re.compile(r"(?m)^<<<<<<< ours\n(.*?)^=======\n(.*?)^>>>>>>> theirs\n", re.S)


def check(ok, message):
    if not ok:
        raise RuntimeError(message)


def resolve(path, ours, theirs, index):
    if path == "common/tls/reality_client.go":
        if index == 0:
            check("classical bool" in ours and "mldsa65Verify *mldsa65.PublicKey" in theirs, "unexpected REALITY fields")
            return ours.rstrip("\n") + "\n\tmldsa65Verify *mldsa65.PublicKey\n"
        if index == 1:
            check("var classical bool" in ours and "var config Config = " in ours, "unexpected classical key share")
            check("var verify *mldsa65.PublicKey" in theirs, "missing ML-DSA verification")
            verification = theirs[:theirs.index("var config Config =")]
            return ours[:ours.index("var config Config =")] + verification + (
                "var config Config = &RealityClientConfig{"
                "ctx, uClient.(*UTLSClientConfig), publicKey, shortID, classical, verify}\n"
            )
        check("e.classical" in ours and "e.mldsa65Verify" in theirs, "unexpected REALITY clone")
        return ours.rstrip("\n") + "\n\t\te.mldsa65Verify,\n"
    if path == "option/tls.go":
        check("KeyShare" in ours and "Mldsa65Verify" in theirs, "unexpected TLS options")
        return ours.rstrip("\n") + "\n\tMldsa65Verify string " + chr(96) + 'json:"mldsa65_verify,omitempty"' + chr(96) + "\n"
    if path == "protocol/vless/outbound.go":
        if index == 0:
            check("h.dialServer(ctx)" in theirs and "h.dialer.DialContext" in ours, "unexpected preconnect dial branch")
            return theirs
        if "context.AfterFunc(ctx" in ours and "h.encryption.Handshake" in theirs:
            return ours
        check("tls.ClientHandshake" in ours and "tls.ClientHandshake" in theirs and
              "rawConn" in ours, "unexpected TLS handshake overlap")
        return ours
    raise RuntimeError("unknown conflict " + path)


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: rebase-panel-protection.py SINGBOX_CHECKOUT PATCH")
    directory, patch = Path(sys.argv[1]).resolve(), Path(sys.argv[2]).resolve()
    check(directory.is_dir() and patch.is_file(), "missing Sing-box or overlay")
    completed = subprocess.run(["git", "apply", "--3way", "--index", str(patch)],
                               cwd=directory, capture_output=True, text=True)
    unmerged = subprocess.check_output(
        ["git", "diff", "--name-only", "--diff-filter=U"], cwd=directory, text=True
    ).splitlines()
    check(set(unmerged) == set(REVIEWED),
          "Unexpected protection overlap: " + repr(unmerged) + "\n" + completed.stdout + completed.stderr)
    for path, expected_count in REVIEWED.items():
        target = directory / path
        source = target.read_text()
        overlaps = list(MARKERS.finditer(source))
        check(len(overlaps) == expected_count, "Unexpected conflict count: " + path + " got " + str(len(overlaps)) + " fragments " + repr([(m.group(1)[:95], m.group(2)[:95]) for m in overlaps]))
        counter = [0]
        def replacement(match):
            index = counter[0]
            counter[0] += 1
            return resolve(path, match.group(1), match.group(2), index)
        updated = MARKERS.sub(replacement, source)
        check("<<<<<<<" not in updated and ">>>>>>>" not in updated, "Unresolved conflict: " + path)
        target.write_text(updated)
    subprocess.run(["gofmt", "-w", *REVIEWED], cwd=directory, check=True)
    subprocess.run(["git", "add", "-A", "--", "common/tls", "option", "protocol/vless"],
                   cwd=directory, check=True)
    check(not subprocess.check_output(["git", "diff", "--name-only", "--diff-filter=U"],
                                      cwd=directory, text=True).strip(), "Unmerged files")
    print("Rebased REALITY ML-DSA and classical mode, Vision preconnect, encryption cancellation.")


if __name__ == "__main__":
    main()
