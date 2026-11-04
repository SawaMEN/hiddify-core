#!/usr/bin/env python3
"""Rebase the panel TCP inbound FinalMask wrapper onto the current Masks API."""
from pathlib import Path
import sys

root = Path(sys.argv[1])
source = root / "common/listener/listener_tcp.go"
text = source.read_text()
before = """\tif masks != nil && masks.TCP != nil {
\t\traw := tcpListener
\t\ttcpListener, err = masks.TCP.WrapListener(raw)
\t\tif err != nil {
\t\t\traw.Close()
\t\t\treturn nil, err
\t\t}
\t}"""
after = """\tif masks != nil && masks.HasTCP() {
\t\ttcpListener = &xuiFinalMaskListener{Listener: tcpListener, masks: masks}
\t}"""
if text.count(before) != 1:
    raise SystemExit("Unexpected panel TCP finalmask patch: review before rebasing")
source.write_text(text.replace(before, after))
wrapper = root / "common/listener/xui_panel_finalmask_listener.go"
if wrapper.exists():
    raise SystemExit("FinalMask wrapper already exists; manual review required")
wrapper.write_text("""// Panel inbound final_mask compatibility with the current Xray mask pipeline.
package listener

import (
    "net"
    "sync"

    "github.com/sagernet/sing-box/hiddify/finalmask"
)

// Delay server mask wrapping until the connection's first I/O; the listener's
// Accept loop must not block on an individual client's mask handshake.
type xuiFinalMaskListener struct {
    net.Listener
    masks *finalmask.Masks
}

func (l *xuiFinalMaskListener) Accept() (net.Conn, error) {
    raw, err := l.Listener.Accept()
    if err != nil {
        return nil, err
    }
    return &xuiFinalMaskConn{Conn: raw, masks: l.masks}, nil
}

type xuiFinalMaskConn struct {
    net.Conn
    masks  *finalmask.Masks
    once   sync.Once
    mu     sync.Mutex
    masked net.Conn
    err    error
}

func (c *xuiFinalMaskConn) wrapped() (net.Conn, error) {
    c.once.Do(func() {
        wrapped, err := c.masks.WrapConnServer(c.Conn)
        c.mu.Lock()
        c.masked, c.err = wrapped, err
        c.mu.Unlock()
    })
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.masked, c.err
}

func (c *xuiFinalMaskConn) Read(buf []byte) (int, error) {
    conn, err := c.wrapped()
    if err != nil {
        return 0, err
    }
    return conn.Read(buf)
}

func (c *xuiFinalMaskConn) Write(buf []byte) (int, error) {
    conn, err := c.wrapped()
    if err != nil {
        return 0, err
    }
    return conn.Write(buf)
}

func (c *xuiFinalMaskConn) Close() error {
    c.mu.Lock()
    wrapped := c.masked
    c.mu.Unlock()
    if wrapped != nil {
        return wrapped.Close()
    }
    return c.Conn.Close()
}
""")
