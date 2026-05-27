---
active: true
iteration: 1
max_iterations: 0
completion_promise: "ALL TASKS COMPLETE"
started_at: "2026-05-27T20:12:09Z"
---

Read .plan/TASKS.md and .plan/PLAN.md. Find all pending tasks whose dependencies are satisfied. Select the set you are confident can run in parallel without file conflicts. Dispatch one implementer subagent per task (in parallel), then one reviewer subagent per task. Fix any blocking review issues. Update each approved task status to `done` in .plan/TASKS.md. When ALL tasks are `done` and every Definition of Done criterion from .plan/PLAN.md is verified, output: <promise>ALL TASKS COMPLETE</promise>
