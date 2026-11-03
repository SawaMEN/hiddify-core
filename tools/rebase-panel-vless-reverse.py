#!/usr/bin/env python3
"""Rebase 3x-ui's VLESS reverse portal onto new sing-box VLESS APIs.

Apply identical original reverse bridge code to disposable panel checkout;
port only the VLESS surface and lifecycle changed upstream.
"""
import pathlib
import re
import subprocess
import sys

EXPECTED = {"experimental/v2rayapi/server.go", "experimental/v2rayapi/stats.go",
            "option/vless.go", "protocol/vless/inbound.go",
            "protocol/vless/outbound.go", "protocol/vless/xui_reverse.go"}

def replace(path, old, new):
    source = path.read_text()
    if source.count(old) != 1:
        raise RuntimeError(f"{path}: reverse source anchor changed: {old[:70]!r} (matches {source.count(old)})")
    path.write_text(source.replace(old, new))

def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: rebase-panel-vless-reverse.py SINGBOX_DIR REVERSE_PATCH")
    root, patch_path = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
    patch = patch_path.read_text()
    parts = re.split(r"(?=^--- (?:a/|/dev/null))", patch, flags=re.M)
    sections = {}
    for part in parts:
        if not part.strip():
            continue
        match = re.search(r"(?m)^\+\+\+ b/(.+)$", part)
        if not match:
            raise RuntimeError("unrecognized reverse patch section")
        if match.group(1) in sections:
            raise RuntimeError("duplicate reverse patch section")
        sections[match.group(1)] = part
    if set(sections) != EXPECTED:
        raise RuntimeError("reverse patch file set changed")
    for name in ("experimental/v2rayapi/server.go", "experimental/v2rayapi/stats.go"):
        subprocess.run(["git", "apply", "--recount", "-"], input=sections[name],
                       cwd=root, text=True, check=True)

    opts = root / "option/vless.go"
    replace(opts, 'type VLESSUser struct {\n',
            'type VLESSReverseOptions struct {\n\tTag string \x60json:"tag"\x60\n}\n'
            'type VLESSReversePortalOptions struct{}\n\n'
            'type VLESSUser struct {\n\tReverse *VLESSReverseOptions \x60json:"reverse,omitempty"\x60\n')
    replace(opts, 'type VLESSOutboundOptions struct {\n',
            'type VLESSOutboundOptions struct {\n\tReverse *VLESSReverseOptions \x60json:"reverse,omitempty"\x60\n')

    inbound = root / "protocol/vless/inbound.go"
    replace(inbound, '\tN "github.com/sagernet/sing/common/network"\n',
            '\tN "github.com/sagernet/sing/common/network"\n'
            '\t"github.com/sagernet/sing/service"\n')
    replace(inbound, '\tinbound.service = service\n',
            '\treverseUsers := make(map[int]bool)\n'
            '\tseenUUID := make(map[string]bool)\n'
            '\tfor i, user := range options.Users {\n'
            '\t\tif seenUUID[user.UUID] { return nil, E.New("duplicate VLESS UUID") }\n'
            '\t\tseenUUID[user.UUID] = true\n'
            '\t\tif user.Reverse != nil {\n'
            '\t\t\tif user.Reverse.Tag == "" { return nil, E.New("reverse tag is required") }\n'
            '\t\t\treverseUsers[i] = true\n'
            '\t\t\tinbound.references = append(inbound.references, user.Reverse.Tag)\n'
            '\t\t}\n'
            '\t}\n'
            '\tservice.SetReverseHandler(reverseUsers, inbound.newReverseConnection)\n'
            '\tinbound.service = service\n')
    replace(inbound, '\t\t\tinbound.references = []string{options.TLS.Reality.Handshake.Detour}\n',
            '\t\t\tinbound.references = append(inbound.references, options.TLS.Reality.Handshake.Detour)\n')
    replace(inbound,
            'func (h *Inbound) Start(stage adapter.StartStage, scope *adapter.Scope) error {\n'
            '\tif stage != adapter.StartStateStart {\n'
            '\t\treturn nil\n'
            '\t}\n',
            'func (h *Inbound) Start(stage adapter.StartStage, scope *adapter.Scope) error {\n'
            '\tif stage != adapter.StartStateStart { return nil }\n'
            '\tfor _, user := range h.users {\n'
            '\t\tif user.Reverse == nil { continue }\n'
            '\t\tmanager := service.FromContext[adapter.OutboundManager](h.ctx)\n'
            '\t\tif manager == nil { return E.New("reverse outbound manager missing") }\n'
            '\t\tob, found := manager.Outbound(user.Reverse.Tag)\n'
            '\t\tif !found { return E.New("reverse portal missing: ", user.Reverse.Tag) }\n'
            '\t\tif _, ok := ob.(*reversePortal); !ok { return E.New("reverse tag is not a portal: ", user.Reverse.Tag) }\n'
            '\t}\n')

    outbound = root / "protocol/vless/outbound.go"
    replace(outbound,
            '\toutbound.Register[option.VLESSOutboundOptions](registry, C.TypeVLESS, NewOutbound)\n',
            '\toutbound.Register[option.VLESSOutboundOptions](registry, C.TypeVLESS, NewOutbound)\n'
            '\toutbound.Register[option.VLESSReversePortalOptions](registry, "vless-reverse", NewReversePortal)\n')
    replace(outbound, '\tvision          bool\n',
            '\tvision          bool\n\treverse         *reverseBridge\n')
    replace(outbound, '\treturn outbound, nil\n}\n\nfunc (h *Outbound) DialContext',
            '\tif options.Reverse != nil {\n'
            '\t\tif options.Reverse.Tag == "" { return nil, E.New("reverse tag is required") }\n'
            '\t\tif muxOpts.Enabled { return nil, E.New("reverse cannot use multiplex") }\n'
            '\t\toutbound.reverse = newReverseBridge(ctx, router, logger, options.Reverse.Tag, (*vlessDialer)(outbound))\n'
            '\t}\n'
            '\treturn outbound, nil\n}\n\nfunc (h *Outbound) DialContext')
    replace(outbound,
            'func (h *Outbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {\n',
            'func (h *Outbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {\n'
            '\tif h.reverse != nil { return nil, E.New("reverse-only outbound cannot dial ordinary connections") }\n')
    replace(outbound,
            'func (h *Outbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {\n',
            'func (h *Outbound) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {\n'
            '\tif h.reverse != nil { return nil, E.New("reverse-only outbound cannot dial ordinary connections") }\n')
    replace(outbound, 'func (h *Outbound) Close() error {\n',
            'func (h *Outbound) Start(stage adapter.StartStage, scope *adapter.Scope) error {\n'
            '\tif stage == adapter.StartStatePostStart && h.reverse != nil { h.reverse.start() }\n'
            '\treturn nil\n'
            '}\n'
            'func (h *Outbound) Close() error {\n'
            '\tif h.reverse != nil { h.reverse.close() }\n')
    replace(outbound,
            '\tswitch N.NetworkName(network) {\n\tcase N.NetworkTCP:\n\t\th.logger.InfoContext(ctx, "outbound connection to ", destination)\n',
            '\tif h.reverse != nil && destination.Fqdn == "v1.rvs.cool" && destination.Port == 0 {\n'
            '\t\treturn h.client.DialEarlyReverseConnWithOptions(conn, visionBaseConn, visionCanSplice)\n'
            '\t}\n'
            '\tswitch N.NetworkName(network) {\n\tcase N.NetworkTCP:\n\t\th.logger.InfoContext(ctx, "outbound connection to ", destination)\n')
    raw = sections["protocol/vless/xui_reverse.go"].splitlines()
    source = "\n".join(line[1:] for line in raw if line.startswith("+") and not line.startswith("+++")) + "\n"
    if not source.startswith("package vless\n") or "type reversePortal struct" not in source or "func newReverseBridge(" not in source:
        raise RuntimeError("reverse implementation changed unexpectedly")
    (root / "protocol/vless/xui_reverse.go").write_text(source)
    subprocess.run(["gofmt", "-w", "option/vless.go", "protocol/vless/inbound.go",
                    "protocol/vless/outbound.go", "protocol/vless/xui_reverse.go"],
                   cwd=root, check=True)
    print("Rebased original VLESS reverse portal and bridge to current lifecycle.")


if __name__ == "__main__":
    main()
