"""Validate a manually requested Go module release before creating any tag."""

import os
from pathlib import Path
import re
import subprocess
import sys


VERSION = re.compile(
    r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
)


def validate(version, module, repository, ref, sha, existing_commit=None):
    match = VERSION.fullmatch(version)
    if not match:
        raise ValueError("版本号必须采用 v0.1.0 或 v0.1.0-rc.1 这样的格式")
    prerelease = match[4]
    if prerelease and any(
        part.isdigit() and len(part) > 1 and part.startswith("0")
        for part in prerelease.split(".")
    ):
        raise ValueError("预发布版本中的纯数字标识不能包含前导零")
    if ref != "refs/heads/main":
        raise ValueError("Release 只能从 main 分支运行")
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("发布必须指定完整的提交 SHA")
    major = int(match[1])
    expected = "github.com/" + repository
    if major >= 2:
        expected += f"/v{major}"
    if module != expected:
        raise ValueError(f"版本号与模块路径不匹配，当前版本要求 module {expected}")
    if existing_commit is not None and existing_commit != sha:
        raise ValueError("版本标签已指向其他提交，请使用新的版本号")
    return {"version": version, "module": module, "prerelease": str(bool(prerelease)).lower()}


def main():
    declaration = re.search(r'^module[ \t]+("[^"\n]+"|\S+)', Path("go.mod").read_text(), re.M)
    if declaration is None:
        raise ValueError("go.mod 缺少 module 声明")
    args = (
        os.environ["RELEASE_VERSION"],
        declaration[1].strip('"'),
        os.environ["GITHUB_REPOSITORY"],
        os.environ["GITHUB_REF"],
        os.environ["GITHUB_SHA"],
    )
    outputs = validate(*args)
    tag = "refs/tags/" + outputs["version"]
    exists = subprocess.run(["git", "show-ref", "--verify", "--quiet", tag])
    if exists.returncode == 0:
        commit = subprocess.check_output(["git", "rev-parse", "--verify", tag + "^{commit}"], text=True).strip()
        validate(*args, existing_commit=commit)
    elif exists.returncode != 1:
        raise ValueError("无法检查已有版本标签")
    if os.environ.get("GITHUB_OUTPUT"):
        with open(os.environ["GITHUB_OUTPUT"], "a") as output:
            for key, value in outputs.items():
                output.write(f"{key}={value}\n")
    print(f"Validated {outputs['module']}@{outputs['version']} at {args[4]}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, subprocess.CalledProcessError) as error:
        print(f"::error::{error}", file=sys.stderr)
        sys.exit(1)
