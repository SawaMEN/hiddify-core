#!/usr/bin/env python3
"""Apply the panel MASQUE speed broker to the upstream batched-packet transport.

This modifies only the disposable panel build checkout. Fail on unknown upstream
shapes: silently dropping a traffic limit is not permitted.
"""
from pathlib import Path
import sys


def edit(root: Path, relative: str, edits: list[tuple[str, str]]) -> None:
    path = root / relative
    source = path.read_text()
    for original, replacement in edits:
        if source.count(original) != 1:
            raise ValueError(f"MASQUE speed overlay incompatible with {relative}: {original[:80]!r}")
        source = source.replace(original, replacement, 1)
    path.write_text(source)


def apply(root: Path) -> None:
    edit(root, "protocol/masque/server.go", [
        ("\tserver, err := masque.NewServer(masque.ServerOptions{\n",
         "\tserver, err := masque.NewServer(masque.ServerOptions{\n\t\tSpeedScope:      tag,\n"),
    ])
    edit(root, "transport/masque/server.go", [
        ('\ttransportHTTP "github.com/sagernet/sing-box/transport/http"\n',
         '\t"github.com/sagernet/sing-box/internal/xuispeed"\n\ttransportHTTP "github.com/sagernet/sing-box/transport/http"\n'),
        ("type ServerOptions struct {\n",
         "type ServerOptions struct {\n\tSpeedScope      string\n"),
        ("type Server struct {\n",
         "type Server struct {\n\tspeedScope      string\n"),
        ("type serverSession struct {\n",
         "type serverSession struct {\n\tspeed *brokerclient.Client\n"),
        ("\tserver := &Server{\n",
         "\tserver := &Server{\n\t\tspeedScope: options.SpeedScope,\n"),
        ("\tcurrent.user, _ = auth.UserFromContext[string](ctx)\n",
         "\tcurrent.user, _ = auth.UserFromContext[string](ctx)\n"
         "\tcurrent.speed, err = brokerclient.New(\"masque:\" + s.speedScope + \":\" + current.user)\n"
         "\tif err != nil {\n"
         "\t\trequest.Reject(http.StatusServiceUnavailable, nil)\n"
         "\t\treturn\n"
         "\t}\n"
         "\tdefer current.speed.Close()\n"),
        ("func (s *serverSession) handlePacket(buffer *buf.Buffer) {\n",
         "func (s *serverSession) waitDownload(n int) error {\n"
         "\treturn s.speed.Wait(s.session.ctx, 1, n)\n"
         "}\n\n"
         "func (s *serverSession) handlePacket(buffer *buf.Buffer) {\n"
         "\tif err := s.speed.Wait(s.session.ctx, 0, buffer.Len()); err != nil {\n"
         "\t\tbuffer.Release()\n"
         "\t\ts.cancel(err)\n"
         "\t\treturn\n"
         "\t}\n"),
    ])
    edit(root, "transport/masque/session.go", [
        ("\tfor i, buffer := range buffers {\n\t\tdatagram := transportHTTP.PrependContextID(buffer)\n",
         "\tfor i, buffer := range buffers {\n"
         "\t\tif limiter, ok := s.handler.(interface{ waitDownload(int) error }); ok {\n"
         "\t\t\tif err := limiter.waitDownload(buffer.Len()); err != nil {\n"
         "\t\t\t\tbuffer.Release()\n"
         "\t\t\t\tbuf.ReleaseMulti(capsules)\n"
         "\t\t\t\tbuf.ReleaseMulti(buffers[i+1:])\n"
         "\t\t\t\treturn err\n"
         "\t\t\t}\n"
         "\t\t}\n"
         "\t\tdatagram := transportHTTP.PrependContextID(buffer)\n"),
    ])


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: rebase-panel-masque.py SING_BOX_CHECKOUT")
    apply(Path(sys.argv[1]))
