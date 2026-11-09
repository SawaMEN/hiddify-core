#!/usr/bin/env python3
"""Rewrite a disposable 3x-ui build to use pinned native Sing-box sources."""
import re
from pathlib import Path

ROOT = '"$task_source/core/hiddify-sing-box/'

def native_check(path):
    return 'test -f ' + ROOT + path + '" || { echo "Missing native source: ' + path + '" >&2; exit 1; }'

def rewrite(panel):
    path = panel / "tools/build-hiddify-core.sh"
    script = path.read_text()
    def once(old, new):
        nonlocal script
        if script.count(old) != 1:
            raise ValueError("Unexpected native overlay integration point: " + old)
        script = script.replace(old, new, 1)
    start = "# Modify verified dependency copies, never the shared Go module cache.\n"
    stop = 'python3 "$task_root/tools/rebase-panel-protection.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/protection-options.patch"'
    if script.count(start) != 1 or script.count(stop) != 1:
        raise ValueError("Old protection block changed")
    a, b = script.index(start), script.index(stop)
    if a >= b:
        raise ValueError("Invalid protection overlay order")
    required = [
        "replace/utls-native/reality.go",
        "third_party/sing-vmess/vless/xui_reverse_native_test.go",
        "third_party/sing-vmess/vless/xui_vision_options.go",
        "third_party/xray-core/go.mod",
        "common/tls/xui_reality_mldsa_handshake_test.go",
        "protocol/vless/xui_vision_testseed_native_test.go",
    ]
    script = script[:a] + "\n".join(native_check(x) for x in required) + "\n" + script[b:]
    return path, script, once
