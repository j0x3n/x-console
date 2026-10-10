#!/usr/bin/env python3
"""后端测试分组：把有测试的包按用时分到 N 组，每组用时差不多。

CI 的 backend-test 用它。新增的包不用改 CI：没写在 test-weights.txt 里的包按 3 秒算，
自动分进某一组。

用法（在 backend/ 目录）：
  python3 ../scripts/test-shard.py 2 4        # 打印第 2 组（共 4 组）的包，一行一个
  go test -json ./... | python3 ../scripts/test-shard.py --update > ../scripts/test-weights.txt
"""
import json
import subprocess
import sys
from pathlib import Path

DEFAULT_WEIGHT = 3.0
WEIGHTS = Path(__file__).with_name("test-weights.txt")
HEADER = [
    "# 后端测试分组用的包用时（秒）。每行：包路径（相对 backend/）用时。",
    "# 不带 -race 跑 go test -json 量的，粗略就行，不用常更新。",
    "# 新包没写在这里时按 3 秒算。更新：见 scripts/test-shard.py 开头。",
]


def update():
    times = {}
    for line in sys.stdin:
        try:
            e = json.loads(line)
        except ValueError:
            continue
        if "Test" not in e and e.get("Action") in ("pass", "fail") and e.get("Elapsed", 0) > 0:
            pkg = e["Package"].split("/backend/", 1)[-1]
            times[pkg] = e["Elapsed"]
    print("\n".join(HEADER))
    for pkg in sorted(times):
        print(f"{pkg} {max(times[pkg], 0.1):.1f}")


def weights():
    out = {}
    for line in WEIGHTS.read_text().splitlines():
        if line.startswith("#") or not line.strip():
            continue
        pkg, sec = line.split()
        out[pkg] = float(sec)
    return out


def packages():
    fmt = "{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}"
    out = subprocess.run(["go", "list", "-f", fmt, "./..."], check=True, capture_output=True, text=True).stdout
    return [p for p in out.split() if p]


def main():
    if sys.argv[1:] == ["--update"]:
        return update()
    shard, total = int(sys.argv[1]), int(sys.argv[2])
    if not 1 <= shard <= total:
        sys.exit("组号要在 1 到 总组数 之间")
    w = weights()
    items = []
    for p in packages():
        short = p.split("/backend/", 1)[-1]
        items.append((w.get(short, DEFAULT_WEIGHT), p))
    # 先放最重的，每次放进当前最轻的一组
    loads = [0.0] * total
    groups = [[] for _ in range(total)]
    for sec, p in sorted(items, key=lambda x: (-x[0], x[1])):
        i = loads.index(min(loads))
        loads[i] += sec
        groups[i].append(p)
    print("\n".join(sorted(groups[shard - 1])))
    print("各组预计用时（秒）：" + " ".join(f"{x:.0f}" for x in loads), file=sys.stderr)


main()
