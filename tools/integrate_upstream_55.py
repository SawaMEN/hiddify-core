#!/usr/bin/env python3
"""One-shot, reviewed upstream v5.5.0 integration helper (runs only in a PR branch).

The source branch is the exact upstream v5.5.0 snapshot. We merge SawaMEN/main
into it, keeping upstream API changes and fork integrations. This helper is
removed from the merge commit.
"""
import json
import pathlib
import re
import subprocess
import sys


def run(*args, input=None, check=True):
    return subprocess.run(args, input=input, text=True, capture_output=True, check=check)


def git_show(revision, path):
    return run("git", "show", f"{revision}:{path}").stdout


def resolve_file(path, side):
    p = pathlib.Path(path)
    source = p.read_text()
    result = []
    lines = source.splitlines(keepends=True)
    index = 0
    conflicts = 0
    while index < len(lines):
        if not lines[index].startswith("<<<<<<< "):
            if lines[index].startswith(("=======", ">>>>>>> ")):
                raise RuntimeError(f"Unexpected conflict separator in {path}:{index+1}")
            result.append(lines[index])
            index += 1
            continue
        conflicts += 1
        index += 1
        ours = []
        theirs = []
        while index < len(lines) and not lines[index].startswith("======="):
            if lines[index].startswith(("<<<<<<< ", "|||||||")):
                raise RuntimeError(f"Nested/diff3 conflict in {path}")
            ours.append(lines[index])
            index += 1
        if index == len(lines):
            raise RuntimeError(f"Unterminated ours block in {path}")
        index += 1
        while index < len(lines) and not lines[index].startswith(">>>>>>> "):
            theirs.append(lines[index])
            index += 1
        if index == len(lines):
            raise RuntimeError(f"Unterminated theirs block in {path}")
        index += 1
        result.extend(ours if side == "upstream" else theirs)
    if conflicts == 0:
        raise RuntimeError(f"No conflicts found in expected file {path}")
    p.write_text("".join(result))
    print(f"Resolved {path} ({conflicts} chunks, preferred {side})")


def merge_go_modules():
    # The latest upstream module graph provides Go 1.27.3 and the new RPC/transport
    # requirements. Carry over fork-specific requirements and *all* fork replacements.
    fork_mod = pathlib.Path("/tmp/hiddify-fork.go.mod")
    fork_mod.write_text(git_show("origin/main", "go.mod"))
    fork = json.loads(run("go", "mod", "edit", "-json", f"-modfile={fork_mod}").stdout)
    upstream = json.loads(run("go", "mod", "edit", "-json", "-modfile=go.mod").stdout)
    existing = {x["Path"] for x in upstream.get("Require", [])}
    added = []
    for req in fork.get("Require", []):
        path, version = req["Path"], req["Version"]
        if path not in existing:
            run("go", "mod", "edit", f"-require={path}@{version}")
            added.append(path)
    for replace in fork.get("Replace", []):
        old, new = replace["Old"], replace["New"]
        lhs = old["Path"] + (("@" + old["Version"]) if old.get("Version") else "")
        rhs = new["Path"] + (("@" + new["Version"]) if new.get("Version") else "")
        run("go", "mod", "edit", f"-replace={lhs}={rhs}")
    print(f"Preserved {len(added)} fork-only module requirements and {len(fork.get('Replace',[]))} replacements")
    # Hiddify 5.5 upstream expects a MasterDnsVPN source module that was added
    # only to its own sing-box fork. Do not keep an invalid local replacement
    # when preserving SawaMEN's pinned, independently maintained fork.
    # Once the custom fork integrates MasterDnsVPN, this guard can be removed.
    run("go", "mod", "edit", "-dropreplace=masterdnsvpn-go")
    run("go", "mod", "edit", "-droprequire=masterdnsvpn-go")
    # Keep valid checksums from both upstream and the custom fork; go mod tidy
    # normalizes this after exact dependency selection.
    sums = set()
    for rev in ("HEAD", "origin/main"):
        for line in git_show(rev, "go.sum").splitlines():
            if line.strip():
                sums.add(line)
    pathlib.Path("go.sum").write_text("\n".join(sorted(sums)) + "\n")


def main():
    unmerged = run("git", "diff", "--name-only", "--diff-filter=U").stdout.splitlines()
    print("Unmerged paths:", unmerged)
    expected = {
        ".github/workflows/build.yml", "Makefile", "go.mod", "go.sum",
        "hiddify-sing-box", "v2/config/builder.go",
        "v2/hcore/adminbgservice/admin_bg_service.go",
        "v2/hcore/hcore.pb.go", "v2/hcore/hcore_service.pb.go",
        "v2/hcore/hcore_service_grpc.pb.go", "v2/hcore/proxy_info.go",
        "v2/hcore/proxy_info_test.go",
    }
    unexpected = set(unmerged) - expected
    if unexpected:
        raise SystemExit("Unexpected conflicts require human review: " + repr(sorted(unexpected)))
    if not unmerged:
        print("The merge had no conflicts")
        return

    # Keep custom CI release pipelines; upstream's build.yml does not know about
    # VetroffProxy, the 3x-ui panel overlays, and the custom publishing scheme.
    pathlib.Path(".github/workflows/build.yml").write_text(git_show("origin/main", ".github/workflows/build.yml"))

    # Preserve the independently maintained SawaMEN Sing-box fork, not the official
    # Hiddify gitlink which would remove proprietary/compatibility extensions.
    sing = run("git", "rev-parse", "origin/main:hiddify-sing-box").stdout.strip()
    run("git", "update-index", "--add", "--cacheinfo", f"160000,{sing},hiddify-sing-box")
    print("Pinned custom hiddify-sing-box revision:", sing)

    # The new upstream Makefile tag system is used by the OpenWrt 5.5 build,
    # but the fork's native regression CI needs its dedicated no-Cronet tag list.
    resolve_file("Makefile", "upstream")
    mk = pathlib.Path("Makefile")
    mkt = mk.read_text()
    anchor = "TAGS=$(call tags,$(COMMON_TAGS) $(NAIVE_TAGS))"
    if anchor not in mkt:
        raise RuntimeError("Upstream tag configuration was not merged")
    mkt = mkt.replace(anchor, anchor + "\n# Test native APIs without the Cronet linker.\nNATIVE_TEST_COMMA := ,\nNATIVE_TEST_TAGS=$(filter-out with_naive_outbound,$(subst $(NATIVE_TEST_COMMA), ,$(TAGS)))", 1)
    mk.write_text(mkt)

    # Start from the complete current upstream graph, then restore local modules.
    pathlib.Path("go.mod").write_text(git_show("HEAD", "go.mod"))
    merge_go_modules()

    # New upstream config/router and service signatures must survive the merge.
    # Keep Git's nonconflicting fork edits outside these conflict regions.
    for path in (
        "v2/config/builder.go",
        "v2/hcore/adminbgservice/admin_bg_service.go",
        "v2/hcore/proxy_info.go",
    ):
        resolve_file(path, "upstream")

    # The protobuf sources merge cleanly (including custom NetworkProbe RPCs).
    # Bindings are generated from their merged schema later in CI; do not try
    # to hand-merge generated descriptors which often become corrupt.
    for path in (
        "v2/hcore/hcore.pb.go",
        "v2/hcore/hcore_service.pb.go",
        "v2/hcore/hcore_service_grpc.pb.go",
    ):
        pathlib.Path(path).write_text(git_show("HEAD", path))

    # Both sides added different tests under the same name. Retain both.
    original_tests = git_show("origin/main", "v2/hcore/proxy_info_test.go")
    pathlib.Path("v2/hcore/proxy_info_fork_test.go").write_text(original_tests)
    pathlib.Path("v2/hcore/proxy_info_test.go").write_text(git_show("HEAD", "v2/hcore/proxy_info_test.go"))

    run("git", "add", "-A")
    remaining = run("git", "diff", "--name-only", "--diff-filter=U").stdout
    if remaining:
        raise SystemExit("Unresolved paths:\n" + remaining)
    print("All conflicts resolved without replacing custom nonconflicting source files.")


if __name__ == "__main__":
    main()
