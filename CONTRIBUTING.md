# Contributing to Ballast

Ballast is developed solo with AI coding agents, but run with the discipline
of a real open-source project: every change traces back to a spec, and every
change lands through a pull request.

## Workflow

1. **Find or create a spec.** Features live in `specs/<NNN-feature-name>/`,
   generated via `/speckit-specify`. Don't write code against requirements
   that don't exist in a spec yet.
2. **Check the constitution.** `.specify/memory/constitution.md` holds the
   project's non-negotiables (tech stack, protocol correctness, test-first
   requirements for the upload engine, security-at-rest, scope boundaries).
   A plan or task that conflicts with it needs to change, or needs an
   explicit, documented justification — not a silent exception.
3. **Work from tasks, not vibes.** Each feature's `tasks.md` (from
   `/speckit-tasks`) breaks the plan into concrete, dependency-ordered
   tasks, each mapped to a GitHub issue via `/speckit-taskstoissues`.
4. **One pull request per task.** Small, reviewable diffs — even with a
   single maintainer, this keeps history readable and makes it possible to
   bisect a regression back to one task.
5. **Tests before implementation for the upload path.** Per the
   constitution's Test-First principle: session/offset/resume logic, retry
   classification, and adaptive chunk-sizing/concurrency algorithms must
   have failing tests before the implementation lands.
6. **Reliability gates are acceptance criteria, not aspirations.** A PR
   touching the upload path should be checked against the relevant Success
   Criteria in the problem statement before merge (resume-within-2s,
   crash/reboot recovery, behavior under simulated packet loss, etc.),
   not just unit-tested in isolation.

## Reporting issues

Open a GitHub issue. If it's a bug in upload reliability, include: file
size, network conditions if known, and whether the upload was resumable or
in-progress at the time of failure.

## Code of Conduct

Be respectful and constructive. This is a small project — treat it kindly.
