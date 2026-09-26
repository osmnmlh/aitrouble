#!/usr/bin/env bash
# setup_github.sh — aitrouble GitHub repository bootstrapper
#
# Usage:
#   chmod +x setup_github.sh
#   ./setup_github.sh
#
# Prerequisites:
#   - gh CLI installed and authenticated (gh auth login)
#   - Must be run from inside the aitrouble git repository
#   - The GitHub repo must already exist (gh repo create aitrouble --public)

set -euo pipefail

REPO="osmnmlh/aitrouble"
TODAY=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  aitrouble GitHub Setup Script"
echo "  Target repo: https://github.com/${REPO}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# ─── 1. Labels ────────────────────────────────────────────────────────────────
echo "[1/3] Creating labels..."

create_label() {
  local name="$1" color="$2" desc="$3"
  gh label create "$name" --color "$color" --description "$desc" \
    --repo "$REPO" --force
  echo "  ✓ label: $name"
}

# Domain labels
create_label "domain:config"   "0075CA" "Config reading, .env, shell env, precedence"
create_label "domain:network"  "006B75" "DNS, TCP, TLS, HTTP probes"
create_label "domain:provider" "E4E669" "OpenAI and OpenAI-compatible provider probes"
create_label "domain:mcp"      "D93F0B" "Local MCP process probes"
create_label "domain:engine"   "5319E7" "Core diagnostic engine and correlation layer"

# Type labels
create_label "type:spike"      "FEF2C0" "Time-boxed investigation or proof-of-concept"
create_label "type:feature"    "84B6EB" "New feature or enhancement"
create_label "type:bug"        "D73A4A" "Something is broken"
create_label "type:docs"       "0075CA" "Documentation improvement"
create_label "type:test"       "CFD3D7" "Test coverage or fixture improvement"

# Community labels
create_label "good first issue" "7057FF" "Good for newcomers"
create_label "help wanted"      "008672" "Extra attention is needed"

echo ""
echo "[2/3] Creating milestones..."

create_milestone() {
  local title="$1" desc="$2" due="$3"
  gh api "repos/${REPO}/milestones" \
    -f title="$title" \
    -f description="$desc" \
    -f due_on="$due" \
    --silent || true
  echo "  ✓ milestone: $title"
}

# Due dates: 1/2/3/4 weeks from now
DUE_M0=$(date -u -d "+7 days"  +"%Y-%m-%dT23:59:59Z" 2>/dev/null || date -u -v+7d  +"%Y-%m-%dT23:59:59Z")
DUE_M1=$(date -u -d "+14 days" +"%Y-%m-%dT23:59:59Z" 2>/dev/null || date -u -v+14d +"%Y-%m-%dT23:59:59Z")
DUE_M2=$(date -u -d "+21 days" +"%Y-%m-%dT23:59:59Z" 2>/dev/null || date -u -v+21d +"%Y-%m-%dT23:59:59Z")
DUE_M3=$(date -u -d "+28 days" +"%Y-%m-%dT23:59:59Z" 2>/dev/null || date -u -v+28d +"%Y-%m-%dT23:59:59Z")

create_milestone \
  "M0: Spike Verification" \
  "Time-boxed spikes to prove the three core assumptions: EffectiveConfig precedence, deterministic probes, and safe secret handling." \
  "$DUE_M0"

create_milestone \
  "M1: Core Engine and Effective Config" \
  "Implement the config reading engine with full precedence hierarchy: shell env > .env file > project config." \
  "$DUE_M1"

create_milestone \
  "M2: Network and Provider Probes" \
  "Implement DNS, TCP, TLS, HTTP, and OpenAI-compatible provider probes with deterministic evidence output." \
  "$DUE_M2"

create_milestone \
  "M3: Local MCP and Doctor CLI v0.1" \
  "Implement local MCP process probe and the top-level 'aitrouble doctor' command with cross-layer diagnosis." \
  "$DUE_M3"

echo ""
echo "[3/3] Creating issues..."

create_issue() {
  local title="$1" labels="$2" milestone="$3" body_file="$4"
  gh issue create \
    --repo "$REPO" \
    --title "$title" \
    --label "$labels" \
    --milestone "$milestone" \
    --body-file "$body_file"
  echo "  ✓ issue: $title"
}

create_issue \
  "[Spike] Verify EffectiveConfig precedence between shell env and .env" \
  "type:spike,domain:config" \
  "M0: Spike Verification" \
  ".github/issue_bodies/issue1.md"

create_issue \
  "[Spike] Deterministic TCP connection refused and timeout probe" \
  "type:spike,domain:network" \
  "M0: Spike Verification" \
  ".github/issue_bodies/issue2.md"

create_issue \
  "[Spike] Safe OpenAI-compatible models probe without leaking secrets" \
  "type:spike,domain:provider" \
  "M0: Spike Verification" \
  ".github/issue_bodies/issue3.md"

create_issue \
  "[Spike] Cross-layer correlation prototype in spike/main.go" \
  "type:spike,domain:engine" \
  "M0: Spike Verification" \
  ".github/issue_bodies/issue4.md"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  ✅ Setup complete!"
echo ""
echo "  Next steps:"
echo "  1. git checkout -b spike/m0-effective-config"
echo "  2. Implement spike (see Issue #1)"
echo "  3. Open a PR: gh pr create --fill"
echo ""
echo "  Repo: https://github.com/${REPO}"
echo "  Issues: https://github.com/${REPO}/issues"
echo "  Milestones: https://github.com/${REPO}/milestones"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
