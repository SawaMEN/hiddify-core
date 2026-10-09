package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/hiddify/hiddify-core/cmd/internal/build_shared"
)

func main() {
	build_shared.FindSDK()
	compiler := filepath.Join(os.Getenv("ANDROID_NDK_HOME"), "toolchains", "llvm", "prebuilt", runtime.GOOS+"-x86_64", "bin", "aarch64-linux-android24-clang")
	output := filepath.Join("bin", "libhiddify-root.so")
	tags := "with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api,with_grpc,with_awg,tfogo_checklinkname0,with_naive_outbound,with_conntrack,with_embedded_tor,with_openvpn,with_openconnect"
	command := exec.Command("go", "build", "-trimpath", "-buildmode=pie", "-tags", tags, "-ldflags=-s -w -checklinkname=0 -extldflags=-Wl,-z,max-page-size=16384", "-o", output, "./cmd/android-root")
	command.Env = append(os.Environ(), "GOOS=android", "GOARCH=arm64", "CGO_ENABLED=1", "CC="+compiler)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
