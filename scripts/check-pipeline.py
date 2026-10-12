import json
import re
from pathlib import Path

root = Path(__file__).resolve().parent.parent
ci = (root / ".github/workflows/ci.yml").read_text(encoding="utf-8")
assert "RACE_INPUT: ${{ toJSON(inputs.race) != 'false' }}" in ci
for value, expected in [(False, False), (True, True), (None, True)]:
    assert (json.dumps(value) != "false") == expected

dockerfile = (root / "deploy/Dockerfile").read_text(encoding="utf-8")
stages = re.split(r"(?m)^FROM ", dockerfile)[1:]
agent_compile = next(stage for stage in stages if "AS agent-compile\n" in stage)
server_compile = next(stage for stage in stages if "AS server-compile\n" in stage)
assert "ARG VERSION" not in agent_compile
assert "ARG VERSION" not in server_compile
assert "COPY backend/ ./" not in agent_compile
assert "linux/amd64 linux/arm64 windows/amd64 windows/arm64" in agent_compile
assert "windowsgui" in agent_compile
assert "sha256sum" in dockerfile

deploy = (root / ".github/workflows/deploy.yml").read_text(encoding="utf-8")
agents = deploy.split("  agents:\n", 1)[1].split("\n  #", 1)[0]
assert "go build" not in agents
assert "needs: [test, image]" in agents
assert "x-console-agent-linux-amd64" in agents
assert "x-console-agent-windows-amd64" in agents
print("构建流程回归检查通过：race 默认值、版本缓存分层、平台代理和产物复用。")
