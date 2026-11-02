#!/usr/bin/env python3
"""Rebase 3x-ui transport/balancer overlay without replacing the newer mKCP engine.

Fail closed if the target API or a panel patch changes.  Changes are restricted
to the disposable sing-box checkout used for the panel build.
"""
import pathlib
import re
import subprocess
import sys

APPLY = {
    "option/group.go",
    "common/urltest/urltest.go",
    "transport/v2rayxhttp/client.go",
    "transport/v2rayxhttp/server.go",
    "transport/v2rayxhttp/xpadding.go",
    "transport/v2rayxhttp/writer.go",
    "transport/v2rayxhttp/dialer.go",
}
EXPECTED = {
    "option/v2ray_transport.go",
    "option/group.go",
    "transport/v2ray/transport.go",
    "protocol/group/selector.go",
    "protocol/group/xui_balancer.go",
    "common/urltest/urltest.go",
    "transport/v2raykcp/config.go",
    "transport/v2raykcp/client.go",
    "transport/v2raykcp/server.go",
    "transport/v2rayxhttp/client.go",
    "transport/v2rayxhttp/server.go",
    "transport/v2rayxhttp/xpadding.go",
    "transport/v2rayxhttp/writer.go",
    "transport/v2rayxhttp/dialer.go",
    "transport/v2rayxhttp/upload_queue.go",
}

def replace_once(path, old, new):
    source = path.read_text()
    if source.count(old) != 1:
        raise RuntimeError(f"{path}: expected exactly one anchor, found {source.count(old)}")
    path.write_text(source.replace(old, new))

def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: rebase-panel-transport.py SINGBOX_DIR PANEL_PATCH")
    root, patch_path = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
    if not root.is_dir() or not patch_path.is_file():
        raise RuntimeError("missing sing-box checkout or panel patch")
    patch = patch_path.read_text()
    chunks = re.split(r"(?=^--- (?:a/|/dev/null))", patch, flags=re.M)
    sections = {}
    for part in chunks:
        if not part.strip():
            continue
        match = re.search(r"(?m)^\+\+\+ b/(.+)$", part)
        if not match:
            raise RuntimeError("unrecognized panel transport patch section")
        key = match.group(1)
        if key in sections:
            raise RuntimeError("duplicate transport patch target: " + key)
        sections[key] = part
    if set(sections) != EXPECTED:
        raise RuntimeError("panel transport patch target list changed")

    # Apply all non-conflicting feature and hardening hunks from the original
    # patch. This includes URL test HTTP method support, nested XHTTP client,
    # padding validation, writer accounting and race-safe WaitReadCloser.
    for name in sorted(APPLY):
        subprocess.run(["git", "apply", "--recount", "-"], cwd=root,
                       input=sections[name], text=True, check=True)
        print("Rebased original panel transport patch:", name)

    opt = root / "option/v2ray_transport.go"
    source = opt.read_text()
    # mKCP is already first-class, including its constructor registration.
    # Preserve the newer, tested with_kcp implementation rather than introducing
    # circular imports or overwriting the fork's improved session engine.
    required = ("KCPOptions         V2RayKCPOptions", "V2RayTransportTypeKCPAlias",
                "type V2RayXHTTPDownloadOptions struct")
    if "type V2RayKCPOptions struct" not in (root / "option/h_v2ray_kcp.go").read_text():
        raise RuntimeError("native mKCP options are missing")
    if any(value not in source for value in required):
        raise RuntimeError("new fork mKCP/XHTTP API moved")
    replace_once(opt,
        'type V2RayXHTTPDownloadOptions struct {\n',
        'type V2RayXHTTPDownloadOptions struct {\n'
        '\tDownload *V2RayXHTTPDownloadOptions \x60json:"download,omitempty"\x60\n'
        '\tMode string \x60json:"mode,omitempty"\x60\n')
    replace_once(opt,
        '\tif c.Download != nil {\n'
        '\t\terr = checkV2RayXHTTPBaseOptions(c.Mode, &c.Download.V2RayXHTTPBaseOptions)\n'
        '\t\tif err != nil {\n'
        '\t\t\treturn err\n'
        '\t\t}\n'
        '\t}\n'
        '\treturn nil\n'
        '}\n\nfunc checkV2RayXHTTPBaseOptions',
        '\t_, err = c.ResolveDownload()\n'
        '\treturn err\n'
        '}\n\nfunc (c *V2RayXHTTPOptions) ResolveDownload() (*V2RayXHTTPDownloadOptions, error) {\n'
        '\tif c.Download != nil && c.Mode == "stream-one" {\n'
        '\t\treturn nil, E.New("stream-one cannot use a separate download")\n'
        '\t}\n'
        '\tseen := map[*V2RayXHTTPDownloadOptions]bool{}\n'
        '\tvar last *V2RayXHTTPDownloadOptions\n'
        '\tfor node, depth := c.Download, 1; node != nil; node, depth = node.Download, depth+1 {\n'
        '\t\tif depth > 8 || seen[node] {\n'
        '\t\t\treturn nil, E.New("download nesting exceeds 8 or contains a cycle")\n'
        '\t\t}\n'
        '\t\tseen[node] = true\n'
        '\t\tswitch node.Mode {\n'
        '\t\tcase "", "auto", "packet-up", "stream-up", "stream-one":\n'
        '\t\tdefault:\n'
        '\t\t\treturn nil, E.New("unsupported download mode: ", node.Mode)\n'
        '\t\t}\n'
        '\t\tif node.Server == "" || node.ServerPort == 0 {\n'
        '\t\t\treturn nil, E.New("download server and port required")\n'
        '\t\t}\n'
        '\t\tif err := checkV2RayXHTTPBaseOptions(node.Mode, &node.V2RayXHTTPBaseOptions); err != nil {\n'
        '\t\t\treturn nil, err\n'
        '\t\t}\n'
        '\t\tlast = node\n'
        '\t}\n'
        '\treturn last, nil\n'
        '}\n\nfunc checkV2RayXHTTPBaseOptions')
    # The newer fork preserves native KCP and a separately named Xray legacy
    # engine under with_kcp, supporting the original panel interop test.
    # Do not overwrite these with duplicate older KCP implementations.
    legacy = (root / "transport/v2raykcp/legacy_config.go").read_text()
    if "func buildOptions(" not in legacy or "mkcp-legacy" not in legacy:
        raise RuntimeError("panel Xray mKCP legacy support not found")

    sel = root / "protocol/group/selector.go"
    replace_once(sel, 'type Selector struct {\n',
                 'type Selector struct {\n\tbalance *balanceState\n')
    replace_once(sel,
                 'outbound.NewAdapter(C.TypeSelector, tag, nil, options.Outbounds)',
                 'outbound.NewAdapter(C.TypeSelector, tag, nil, selectorDependencies(options))')
    replace_once(sel,
                 '\tif len(outbound.tags) == 0 {\n',
                 '\tif options.Balancer != nil {\n'
                 '\t\tvar err error\n'
                 '\t\toutbound.balance, err = newBalanceState(ctx, *options.Balancer)\n'
                 '\t\tif err != nil { return nil, err }\n'
                 '\t}\n'
                 '\tif len(outbound.tags) == 0 {\n')
    replace_once(sel,
                 'func (s *Selector) Network() []string {\n',
                 'func (s *Selector) Network() []string {\n'
                 '\tif s.balance != nil { return []string{N.NetworkTCP, N.NetworkUDP} }\n')
    replace_once(sel,
                 '\tfor i, tag := range s.tags {\n\t\tdetour, loaded := s.outbound.Outbound(tag)\n',
                 '\tif s.balance != nil && s.balance.config.Fallback != "" {\n'
                 '\t\ttag := s.balance.config.Fallback\n'
                 '\t\tdetour, ok := s.outbound.Outbound(tag)\n'
                 '\t\tif !ok { return E.New("balancer fallback not found: ", tag) }\n'
                 '\t\ts.outbounds[tag] = detour\n'
                 '\t}\n'
                 '\tfor i, tag := range s.tags {\n\t\tdetour, loaded := s.outbound.Outbound(tag)\n')
    replace_once(sel,
                 'func (s *Selector) References() []string {\n',
                 'func (s *Selector) References() []string {\n'
                 '\tif s.balance != nil { return selectorDependencies(option.SelectorOutboundOptions{Outbounds:s.tags, Balancer: &s.balance.config}) }\n')
    replace_once(sel,
                 'func (s *Selector) SelectOutbound(tag string) bool {\n',
                 'func (s *Selector) SelectOutbound(tag string) bool {\n'
                 '\tif s.balance != nil { return false }\n')
    replace_once(sel,
                 '\tconn, err := s.selected.Load().DialContext(ctx, network, destination)\n',
                 '\ttarget, err := s.pickOutbound()\n'
                 '\tif err != nil { return nil, err }\n'
                 '\tconn, err := target.DialContext(ctx, network, destination)\n')
    replace_once(sel,
                 '\tconn, err := s.selected.Load().ListenPacket(ctx, destination)\n',
                 '\ttarget, err := s.pickOutbound()\n'
                 '\tif err != nil { return nil, err }\n'
                 '\tconn, err := target.ListenPacket(ctx, destination)\n')
    with sel.open("a") as stream:
        stream.write("""
func (s *Selector) pickOutbound() (adapter.Outbound, error) {
    if s.balance == nil {
        target := s.selected.Load()
        if target == nil { return nil, E.New("selector is not started") }
        return target, nil
    }
    tag, err := s.balance.pick(s.tags)
    if err != nil { return nil, err }
    target := s.outbounds[tag]
    if target == nil { return nil, E.New("balancer selected unknown outbound ", tag) }
    s.selected.Store(target)
    return target, nil
}
func (s *Selector) PostStart() error {
    if s.balance != nil { s.balance.postStart(s.outbounds) }
    return nil
}
func (s *Selector) Close() error {
    if s.balance != nil { return s.balance.Close() }
    return nil
}
func selectorDependencies(o option.SelectorOutboundOptions) []string {
    refs := append([]string{}, o.Outbounds...)
    if o.Balancer != nil && o.Balancer.Fallback != "" && !slices.Contains(refs, o.Balancer.Fallback) {
        refs = append(refs, o.Balancer.Fallback)
    }
    return refs
}
""")
    helper = sections["protocol/group/xui_balancer.go"].splitlines()
    extracted = "\n".join(line[1:] for line in helper if line.startswith("+") and not line.startswith("+++")) + "\n"
    if not extracted.startswith("package group\n") or "func newBalanceState(" not in extracted:
        raise RuntimeError("unexpected Xray balancer source")
    (root / "protocol/group/xui_balancer.go").write_text(extracted)
    # Original patch's queue replacement targeted an older, unsynchronized
    # queue. Retain fork's atomic closed state and close-notifier protocol.
    queue = (root / "transport/v2rayxhttp/upload_queue.go").read_text()
    if "readerAccess    sync.Mutex" not in queue or "closing         chan struct{}" not in queue or "closeOnce       sync.Once" not in queue:
        raise RuntimeError("newer XHTTP upload queue hardening is missing")
    subprocess.run(["gofmt", "-w", "option/group.go", "option/v2ray_transport.go",
                    "protocol/group/selector.go", "protocol/group/xui_balancer.go"],
                   cwd=root, check=True)
    print("Rebased Xray balancer + XHTTP enhancements; preserved newer native mKCP and queue")


if __name__ == "__main__":
    main()
