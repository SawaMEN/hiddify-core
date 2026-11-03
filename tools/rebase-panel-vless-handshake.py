#!/usr/bin/env python3
"""Check that VLESS handshake safety overlay is superseded by the newer fork.

The old patch wrapped rawConn.Close around encryption failure but removed its
context cancellation guard.  Retain the fork's newer cancellable implementation
instead of silently undoing it.
"""
import pathlib
import sys

def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: rebase-panel-vless-handshake.py SINGBOX_DIR PANEL_PATCH")
    root, patch_path = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
    patch = patch_path.read_text()
    if not patch.startswith("--- a/protocol/vless/outbound.go\n+++ b/protocol/vless/outbound.go\n"):
        raise RuntimeError("unexpected VLESS handshake overlay target")
    if patch.count("@@") != 4 or patch.count("encryption handshake") < 2:
        raise RuntimeError("VLESS handshake overlay semantics changed")
    source = (root / "protocol/vless/outbound.go").read_text()
    required = [
        "var closeOnce sync.Once",
        "closeOnce.Do(func() { _ = rawConn.Close() })",
        "stopHandshake := context.AfterFunc(ctx, closeRaw)",
        "encryptedConn, handshakeErr := h.encryption.Handshake(rawConn)",
        "closeRaw()",
        'return nil, E.Cause(handshakeErr, "encryption handshake")',
        "conn = encryptedConn",
    ]
    if any(source.count(x) < 2 for x in required):
        raise RuntimeError("VLESS encryption handshake is missing atomic cleanup and cancellation safety")
    print("VLESS handshake safety confirmed; current cancellable implementation retained.")

if __name__ == "__main__":
    main()
