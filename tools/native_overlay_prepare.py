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
    for command, evidence in [
        (stop, "common/tls/xui_reality_mldsa_handshake_test.go"),
        ('python3 "$task_root/tools/rebase-panel-transport.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/transport-balancer.patch"', "protocol/group/xui_balancer_test.go"),
        ('python3 "$task_root/tools/rebase-panel-vless-reverse.py" "$task_source/core/hiddify-sing-box" "$task_root/deploy/hiddify/vless-reverse.patch"', "protocol/vless/xui_reverse_native_test.go"),
        ('git -C "$task_source/core" apply --directory=xui-dependencies/xray-core "$task_root/deploy/hiddify/xray-runtime-compat.patch"', "third_party/xray-core/go.mod"),
    ]:
        if "xui-dependencies/" in command and command not in script:
            continue
        once(command, native_check(evidence))
    tests = {
        "inbound_finalmask_test.go.in": "xui_inbound_finalmask_test.go",
        "http1_options_test.go.in": "option/xui_panel_http1_test.go",
        "clash_user_test.go.in": "experimental/clashapi/trafficontrol/xui_panel_test.go",
        "sniff_override_test.go.in": "route/rule/xui_panel_sniff_test.go",
        "stream_upgrade_test.go.in": "transport/v2rayhttpupgrade/xui_stream_test.go",
        "stream_trojan_test.go.in": "transport/trojan/xui_stream_test.go",
        "explicit_cipher_suites_test.go.in": "common/tls/xui_explicit_ciphers_test.go",
        "vless_handshake_test.go.in": "protocol/vless/xui_handshake_test.go",
        "reverse_vision_test.go.in": "third_party/sing-vmess/vless/xui_reverse_native_test.go",
        "reverse_runtime_test.go.in": "protocol/vless/xui_reverse_native_test.go",
        "xhttp_extension_test.go.in": "transport/v2rayxhttp/xui_upload_queue_native_test.go",
        "balancer_strategy_test.go.in": "protocol/group/xui_balancer_test.go",
        "reality_protection_test.go.in": "common/tls/xui_reality_mldsa_handshake_test.go",
        "vision_preconnect_test.go.in": "protocol/vless/xui_preconnect_test.go",
        "vision_padding_test.go.in": "protocol/vless/xui_vision_testseed_native_test.go",
    }
    once('git -C "$task_source/core" apply --directory=xui-dependencies/sing-vmess "$task_root/deploy/hiddify/sing-vmess-reverse.patch"', native_check("third_party/sing-vmess/vless/xui_reverse_native_test.go"))
    for fixture, evidence in tests.items():
        matches = [line for line in script.splitlines() if
                   ('"$task_root/deploy/hiddify/' + fixture + '"') in line
                   and line.lstrip().startswith("cp ")]
        if len(matches) != 1:
            raise ValueError("Unexpected native test injection: " + fixture)
        once(matches[0], native_check(evidence))
    once('  GOTOOLCHAIN="${target[5]}" go mod edit -replace github.com/metacubex/utls=./xui-dependencies/utls -replace github.com/sagernet/sing-vmess=./xui-dependencies/sing-vmess\n', '')
    once('  GOTOOLCHAIN="${target[5]}" go mod edit -replace github.com/xtls/xray-core=./xui-dependencies/xray-core\n', '')
    if "xui-dependencies" in script:
        raise ValueError("Legacy module cache overlays remain")
    return path, script, once
