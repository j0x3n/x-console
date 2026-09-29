import { describe, expect, it } from "vitest";
import type { GitHubPull, GitHubRun } from "./api";
import {
  filterRepos,
  isRepoName,
  checkTone,
  groupByRepo,
  issuePath,
  latestDefaultRuns,
  mappingProblem,
  parseRepos,
  reviewTone,
  runOutcome,
  sortPulls,
} from "./logic";

function pull(
  repo: string,
  number: number,
  extra: Partial<GitHubPull> = {},
): GitHubPull {
  return {
    repo,
    number,
    title: `PR ${number}`,
    author: "jo",
    url: "",
    headRef: "x",
    baseRef: "main",
    draft: false,
    reviewState: "none",
    checkState: "none",
    issueKeys: [],
    createdAt: "2026-09-20T10:00:00Z",
    updatedAt: "2026-09-20T10:00:00Z",
    ...extra,
  };
}

function run(
  repo: string,
  name: string,
  extra: Partial<GitHubRun> = {},
): GitHubRun {
  return {
    id: 1,
    repo,
    name,
    branch: "main",
    event: "push",
    status: "completed",
    conclusion: "success",
    url: "",
    defaultBranch: true,
    createdAt: "2026-09-20T10:00:00Z",
    updatedAt: "2026-09-20T10:00:00Z",
    ...extra,
  };
}

describe("groupByRepo", () => {
  it("groups and sorts repos, keeping item order", () => {
    const groups = groupByRepo([
      pull("b/x", 1),
      pull("a/y", 2),
      pull("b/x", 3),
    ]);
    expect(groups.map((g) => g.repo)).toEqual(["a/y", "b/x"]);
    expect(groups[1].items.map((p) => p.number)).toEqual([1, 3]);
  });
});

describe("sortPulls", () => {
  it("puts failing PRs first, then newest", () => {
    const sorted = sortPulls([
      pull("a/b", 1, { updatedAt: "2026-09-21T00:00:00Z" }),
      pull("a/b", 2, { updatedAt: "2026-09-22T00:00:00Z" }),
      pull("a/b", 3, {
        checkState: "failure",
        updatedAt: "2026-09-01T00:00:00Z",
      }),
    ]);
    expect(sorted.map((p) => p.number)).toEqual([3, 2, 1]);
  });
});

describe("badges", () => {
  it("maps states to tones", () => {
    expect(checkTone("failure")).toBe("danger");
    expect(checkTone("success")).toBe("ok");
    expect(checkTone("none")).toBe("");
    expect(reviewTone("changes_requested")).toBe("danger");
    expect(reviewTone("pending")).toBe("warn");
  });
  it("describes runs", () => {
    expect(runOutcome({ status: "in_progress", conclusion: "" })).toEqual({
      label: "Running",
      tone: "warn",
    });
    expect(
      runOutcome({ status: "completed", conclusion: "timed_out" }).tone,
    ).toBe("danger");
    expect(
      runOutcome({ status: "completed", conclusion: "cancelled" }).tone,
    ).toBe("");
  });
});

describe("helpers", () => {
  it("builds issue paths", () => {
    expect(issuePath("XC-12")).toBe("/projects/XC/12");
    expect(issuePath("bad")).toBe("/projects");
  });
  it("parses repo lists", () => {
    expect(parseRepos(" acme/app\nacme/lib, ACME/app  \n\n")).toEqual([
      "acme/app",
      "acme/lib",
    ]);
  });
  it("keeps the latest default-branch run per workflow", () => {
    const runs = [
      run("a/b", "CI", { id: 3, conclusion: "failure" }),
      run("a/b", "CI", { id: 2 }),
      run("a/b", "Lint", { id: 1, defaultBranch: false }),
      run("c/d", "CI", { id: 4 }),
    ];
    expect(latestDefaultRuns(runs).map((r) => r.id)).toEqual([3, 4]);
  });
});

describe("mappingProblem", () => {
  it("checks rows", () => {
    expect(mappingProblem([])).toBeNull();
    expect(
      mappingProblem([
        { teamId: "a", projectId: 1 },
        { teamId: "b", projectId: 2 },
      ]),
    ).toBeNull();
    expect(mappingProblem([{ teamId: "", projectId: 1 }])).toMatch(/every row/);
    expect(
      mappingProblem([
        { teamId: "a", projectId: 1 },
        { teamId: "a", projectId: 2 },
      ]),
    ).toMatch(/once/);
    expect(
      mappingProblem([
        { teamId: "a", projectId: 1 },
        { teamId: "b", projectId: 1 },
      ]),
    ).toMatch(/once/);
  });
});

describe("repository picker (B35)", () => {
  it("checks owner/name", () => {
    expect(isRepoName("j0x3n/x-console")).toBe(true);
    expect(isRepoName("org/repo.name_2")).toBe(true);
    expect(isRepoName("x-console")).toBe(false);
    expect(isRepoName("a/b/c")).toBe(false);
    expect(isRepoName("")).toBe(false);
  });

  it("searches names and descriptions", () => {
    const repos = [
      { fullName: "j0x3n/x-console", description: "个人控制台" },
      { fullName: "j0x3n/dotfiles" },
    ];
    expect(filterRepos(repos, "CONSOLE").map((r) => r.fullName)).toEqual([
      "j0x3n/x-console",
    ]);
    expect(filterRepos(repos, "控制台")).toHaveLength(1);
    expect(filterRepos(repos, " ")).toHaveLength(2);
  });
});
