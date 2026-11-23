# AI Agent Contributor Guidelines

Welcome. `teeter` is a Go CLI tool for assembling and managing `AzerothCore-WotLK` mod setups from a manifest. If you are working on this repository as an AI agent, treat this document as your operating contract.

Speed without verification is just fast breakage. Move in small, reversible steps, prove your changes work, and leave the repository cleaner than you found it.

---

## 1. Worktrees & Branching (Non-Negotiable)

Never develop directly on `main` or inside a dirty shared working directory.

* **One task, one worktree:** Create a dedicated Git worktree and branch for each discrete change:

```sh
git fetch origin
git worktree add ../teeter-<topic> \
  -b feat/<short-name> origin/main
cd ../teeter-<topic>
```

* **Stay isolated:** Do not touch unrelated worktrees, uncommitted user changes, or untracked scratch files outside your task.
* **Keep commits small and cohesive:** Commit logical steps with clear messages explaining *why*, not just *what*.
* **Clean up when finished:** Remove your worktree once the branch is merged or abandoned (`git worktree remove ../teeter-<topic>`). Never leave half-finished work stranded in `main`.

---

## 2. Test-Driven & Incremental Execution (Kent Beck Mindset)

* **Red → Green → Refactor:**
1. Write or update a failing test first that captures the desired behavior or bug fix.
2. Write the minimal code needed to make the test pass.
3. Refactor for clarity and simplicity while keeping tests green.


* **Small increments:** Break larger features into thin, end-to-end slices rather than giant speculative edits across many files.
* **Make the impossible or invalid obvious:** Validate inputs and manifest assumptions at boundaries; fail loudly with actionable errors rather than silently guessing.

---

## 3. Craftsmanship & Scope Discipline (Uncle Bob Mindset)

* **Strict scope:** Only change what is needed for the task. Do not reformat unrelated files, rename untouched symbols, or introduce unrequested abstractions.
* **Cohesive modules & clear boundaries:** Keep CLI command parsing, manifest loading/validation, and execution/orchestration logic separated so core logic can be tested without shelling out or mutating real host state unnecessarily.
* **No silent shortcuts:** Never comment out failing tests, weaken assertions to make a suite pass, or leave `TODO` hacks in place of proper handling.
* **Respect project tone & simplicity:** Prefer standard library idioms and straightforward Go over cleverness or heavy external dependencies.

---

## 4. Required Validation Hoops

Do **not** report a task complete until every applicable check below has been run and passed in your worktree:

1. **Formatting & static checks:**
```sh
gofmt -l .
go vet ./...

```


(`gofmt -l .` must return empty; fix formatting with `gofmt -w .`.)
2. **Unit & package tests:**
```sh
go test -race ./...

```


3. **Build check:**
```sh
go build ./...

```


4. **Git hygiene check:**
```sh
git status
git diff --stat

```


Verify there are no unintended edits, leftover debug statements, or untracked artifacts.

---

## 5. Reporting Back

When handing work back, be concise and factual:

* State which branch and worktree you worked in.
* Summarize the behavior changed and any key design trade-offs.
* List the exact validation commands you ran and confirm they passed (or report any remaining failures/blockers honestly).
