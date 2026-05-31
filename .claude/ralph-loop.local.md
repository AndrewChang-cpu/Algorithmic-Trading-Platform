---
active: true
iteration: 1
max_iterations: 0
completion_promise: "ALL TASKS COMPLETE"
started_at: "2026-05-30T03:55:00Z"
---

Read .plan/TASKS.md and .plan/PLAN.md. Find all pending tasks whose dependencies are satisfied. Select the set you are confident can run in parallel without file conflicts. Dispatch one implementer subagent per task (in parallel), then one reviewer subagent per task. Fix any blocking review issues. Update each approved task status to `reviewed` in .plan/TASKS.md. When ALL tasks are `reviewed`, run the integration review (vibe:review on full diff), promote all to `done`, verify DoD, and output: <promise>ALL TASKS COMPLETE</promise>
