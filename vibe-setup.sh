#!/bin/bash
# Vibe Coding Environment Bootstrapper (Linux/macOS)

# ==============================================================================
# 0. OS CHECK
# ==============================================================================
if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OS" == "Windows_NT" || -n "$WSL_DISTRO_NAME" ]]; then
  echo "[ERROR] This script is for Linux/macOS only. Use vibe-setup-windows.ps1 on Windows."
  exit 1
fi

echo "[INFO] Initializing AI Agent Environment in $PWD..."

# ==============================================================================
# 1. UNIVERSAL BASE
# ==============================================================================
echo "[INFO] Configuring Universal Base directories..."

source .env
mkdir -p .vibe/skills

if [ ! -f .vibe/project-context.md ]; then
cat << 'EOF' > .vibe/project-context.md
# Project Context
- Name: [TODO: Project name]
- Stack: [TODO: Languages, frameworks, and key libraries]
- Architecture: [TODO: High-level structure and any notable patterns or constraints]

## Project Architecture & Directory Map
[TODO: Define the explicit folder structure.]

## Anti-Patterns & "Never Do This"
[TODO: List specific practices the agent must strictly avoid.]

## Git & Workflow Standards
[TODO: Define commit message format and PR rules.]

## Definition of Done (DoD)
[TODO: Define the checklist the agent must complete before finishing a task.]

## Useful Project Commands
- Run Development Server: [TODO]
- Build for Production: [TODO]
- Run Test Suite: [TODO]
EOF
echo "[CREATED] .vibe/project-context.md"
fi

if [ ! -f .vibe/mcp-triggers.md ]; then
cat << 'EOF' > .vibe/mcp-triggers.md
# MCP Tool Triggers
- Database/SQL: Use postgres MCP to inspect live schema.
- GitHub/VC: Use github MCP to read issues and draft PRs.
- UI/Browser: Use puppeteer MCP to inspect localhost rendering.
- API/Docs: Use context7 MCP for framework documentation.
- Planning: Use sequential-thinking MCP before writing code.
- Search/Research: Use tavily MCP for web search and real-time information.
EOF
echo "[CREATED] .vibe/mcp-triggers.md"
fi

# ------------------------------------------------------------------------------
# 1b. TASK TRACKING & LESSONS
# ------------------------------------------------------------------------------
echo "[INFO] Initializing task tracking..."

if [ ! -f .vibe/lessons.md ]; then
cat << 'EOF' > .vibe/lessons.md
# Lessons Learned

Review this file at the start of every session. Update it after ANY correction from the user.
Write rules for yourself that prevent the same mistake. Ruthlessly iterate until mistake rate drops.

## Format

**Pattern**: [what happened]
**Rule**: [what to do / not do going forward]
**Why**: [root cause or user reasoning]

---

## Active Lessons

<!-- Add new lessons above this line. Remove lessons that have been internalized into config files. -->
EOF
echo "[CREATED] .vibe/lessons.md"
fi

if [ ! -f .vibe/todo.md ]; then
cat << 'EOF' > .vibe/todo.md
# Task Tracker

## Current Tasks

<!-- Plan first: write checkable items here before starting implementation. -->
<!-- Mark items complete as you go. Add a review section when done. -->

## Completed

<!-- Move completed task blocks here for reference. -->
EOF
echo "[CREATED] .vibe/todo.md"
fi

PROJECT_CONTEXT=$(cat .vibe/project-context.md)
MCP_CONTENT=$(cat .vibe/mcp-triggers.md)

# Shared operational rules block used across all agent configs
read -r -d '' OPS_RULES << 'OPSEOF'
# Operational Rules

## 1. Plan Mode Default
- Enter plan mode for ANY non-trivial task (3+ steps or architectural decisions).
- If something goes sideways, STOP and re-plan immediately — don't keep pushing.
- Use plan mode for verification steps, not just building.
- Write detailed specs upfront to reduce ambiguity.

## 2. Subagent Strategy
- Use subagents liberally to keep main context window clean.
- Offload research, exploration, and parallel analysis to subagents.
- For complex problems, throw more compute at it via subagents.
- One task per subagent for focused execution.

## 3. Self-Improvement Loop
- After ANY correction from the user: update `.vibe/lessons.md` with the pattern.
- Write rules for yourself that prevent the same mistake.
- Ruthlessly iterate on these lessons until mistake rate drops.
- Review `.vibe/lessons.md` at session start for relevant patterns.

## 4. Verification Before Done
- Never mark a task complete without proving it works.
- Diff behavior between main and your changes when relevant.
- Ask yourself: "Would a staff engineer approve this?"
- Run tests, check logs, demonstrate correctness.

## 5. Demand Elegance (Balanced)
- For non-trivial changes: pause and ask "is there a more elegant way?"
- If a fix feels hacky: "Knowing everything I know now, implement the elegant solution."
- Skip this for simple, obvious fixes — don't over-engineer.
- Challenge your own work before presenting it.

## 6. Autonomous Bug Fixing
- When given a bug report: just fix it. Don't ask for hand-holding.
- Point at logs, errors, failing tests — then resolve them.
- Zero context switching required from the user.
- Go fix failing CI tests without being told how.

## 7. Task Management
1. **Plan First**: Write plan to `.vibe/todo.md` with checkable items.
2. **Verify Plan**: Check in before starting implementation.
3. **Track Progress**: Mark items complete as you go.
4. **Explain Changes**: High-level summary at each step.
5. **Document Results**: Add review section to `.vibe/todo.md`.
6. **Capture Lessons**: Update `.vibe/lessons.md` after corrections.

## Core Principles
- **Simplicity First**: Make every change as simple as possible. Impact minimal code.
- **No Laziness**: Find root causes. No temporary fixes. Senior developer standards.
OPSEOF

# ==============================================================================
# 2. CURSOR CONFIGURATION
# ==============================================================================
echo "[INFO] Configuring Cursor..."

mkdir -p .cursor/rules

if [ ! -f .cursor/mcp.json ]; then
cat << 'EOF' > .cursor/mcp.json
{
  "mcpServers": {
    "postgres": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres", "postgresql://localhost:5432/postgres"]
    },
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"]
    },
    "sequential-thinking": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"]
    },
    "puppeteer": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-puppeteer"]
    },
    "context7": {
      "command": "npx",
      "args": ["-y", "@upstash/context7-mcp"]
    },
    "tavily": {
      "command": "npx",
      "args": ["-y", "tavily-mcp"]
    }
  }
}
EOF
  echo "[CREATED] .cursor/mcp.json"
fi

cp .vibe/project-context.md .cursor/rules/000-main-rules.mdc
cp .vibe/mcp-triggers.md .cursor/rules/001-mcp-triggers.mdc

cat << EOF > .cursor/rules/002-operational.mdc
$OPS_RULES

---

## Lessons & Self-Correction
Read \`.vibe/lessons.md\` at the start of each session. After ANY user correction, immediately add the pattern to \`.vibe/lessons.md\`.
EOF

# ==============================================================================
# 3. CLAUDE CODE CONFIGURATION
# ==============================================================================
echo "[INFO] Configuring Claude Code..."

claude mcp remove postgres 2>/dev/null || true
claude mcp remove github 2>/dev/null || true
claude mcp remove sequential-thinking 2>/dev/null || true
claude mcp remove puppeteer 2>/dev/null || true
claude mcp remove context7 2>/dev/null || true
claude mcp remove tavily 2>/dev/null || true

claude mcp add --transport stdio postgres -- npx -y @modelcontextprotocol/server-postgres postgresql://localhost:5432/postgres
claude mcp add --transport stdio github -- npx -y @modelcontextprotocol/server-github
claude mcp add --transport stdio sequential-thinking -- npx -y @modelcontextprotocol/server-sequential-thinking
claude mcp add --transport stdio puppeteer -- npx -y @modelcontextprotocol/server-puppeteer
claude mcp add --scope user --transport stdio context7 -- npx -y @upstash/context7-mcp --api-key $CONTEXT7_API_KEY
claude mcp add --transport stdio tavily -e TAVILY_API_KEY=$TAVILY_API_KEY -- npx -y tavily-mcp
# Note: Ensure GITHUB_PERSONAL_ACCESS_TOKEN, CONTEXT7_API_KEY, and TAVILY_API_KEY are set in your .env

cat << EOF > CLAUDE.md
# Global Agent Instructions
You are an expert software architect. Write clean, secure, and optimized code while strictly adhering to the project context and constraints.

## Primary Directives
1. **Plan Before Coding**: For any task touching >2 files, output an architectural plan first.
2. **Minimal Diff**: Only modify files explicitly required.
3. **Run Checks**: Always run linting and testing commands after making logic changes.
4. **Follow Conventions**: Match the existing code style. Prefer clarity over cleverness.

---

$PROJECT_CONTEXT

## Extended Capabilities
ALWAYS read \`.vibe/mcp-triggers.md\` before executing complex tasks or using external tools.

---

$OPS_RULES

---

$MCP_CONTENT

---

## Lessons & Self-Correction
Read \`.vibe/lessons.md\` at the start of each session. After ANY user correction, immediately add the pattern to \`.vibe/lessons.md\`.
EOF

# ==============================================================================
# 4. GITHUB COPILOT CLI & GENERIC STANDARDS
# ==============================================================================
echo "[INFO] Configuring GitHub Copilot & Generic Standards..."

mkdir -p .github

cat << EOF > .github/copilot-instructions.md
$PROJECT_CONTEXT

## Extended Capabilities
ALWAYS read \`.vibe/mcp-triggers.md\` before executing complex tasks or using external tools.

---

$OPS_RULES

---

$MCP_CONTENT

---

## Lessons & Self-Correction
Read \`.vibe/lessons.md\` at the start of each session. After ANY user correction, immediately add the pattern to \`.vibe/lessons.md\`.
EOF

cp CLAUDE.md AGENTS.md

echo "[SUCCESS] Bootstrapping complete. Core files generated and synchronized."
