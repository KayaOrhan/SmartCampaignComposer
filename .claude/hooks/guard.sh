#!/usr/bin/env bash
# PreToolUse guard. Claude Code sends the tool call as JSON on stdin.
# Exit 2 blocks the call and sends stderr back to the model, so every message
# says what to do instead.
#
# Scope: this catches honest mistakes by an agent (reading .env, skipping hooks),
# not a determined attacker. Layer 2 of 3: rule text in RULES.md, this hook,
# and the secret scan in `make lint`.
set -u

fail_closed() {
  echo "BLOCKED: guard hook could not run safely ($1). Install jq or fix .claude/hooks/guard.sh." >&2
  exit 2
}
deny() { printf 'BLOCKED by guard: %s\n' "$1" >&2; exit 2; }

command -v jq >/dev/null 2>&1 || fail_closed "jq not found"
input=$(cat) || fail_closed "cannot read stdin"
tool=$(printf '%s' "$input" | jq -r '.tool_name // empty') || fail_closed "invalid JSON"
[ -n "$tool" ] || fail_closed "missing tool_name"

# .env style files hold secrets. Templates are safe.
is_secret_path() {
  local base
  base=$(basename -- "$1" | tr '[:upper:]' '[:lower:]')
  case "$base" in
    .env.example|.env.sample|.env.template) return 1 ;;
    .env|.env.*|*.pem|id_rsa|id_ed25519) return 0 ;;
  esac
  return 1
}

case "$tool" in
  Read|Write|Edit|MultiEdit)
    path=$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty')
    if [ -n "$path" ] && is_secret_path "$path"; then
      deny "'$path' may hold secrets and must stay out of AI context. Use .env.example for variable names (RULES.md rules 5 and 7)."
    fi
    exit 0
    ;;
  Bash) ;;
  *) exit 0 ;;
esac

cmd=$(printf '%s' "$input" | jq -r '.tool_input.command // empty')
[ -n "$cmd" ] || exit 0
lc=$(printf '%s' "$cmd" | tr '[:upper:]' '[:lower:]' | tr '\n' ' ')

# Regexes live in variables: inline, characters like ; and | break bash's [[ =~ ]] parser.
sp='[[:space:]]'
sep='[^;&|]*'
verbs='(cat|less|more|head|tail|bat|nl|tac|grep|egrep|rg|sed|awk|cut|sort|xxd|od|strings|base64|source|\.)'
envtok='(\.env(\.[a-z0-9_-]+)?)'
end="([[:space:]\"';|&)]|\$)"

# 1. Commands that read a secret file. "cat .env.example" passes, "cat .env" does not.
read_re="(^|[;&|(]|$sp)$verbs$sp+($sep[[:space:]/\"'=])?$envtok$end"
if [[ $lc =~ $read_re ]]; then
  tok=${BASH_REMATCH[4]}
  case "$tok" in
    .env.example|.env.sample|.env.template) ;;
    *) deny "this command reads '$tok'. Secrets stay out of AI context. Use .env.example for variable names (RULES.md rules 5 and 7)." ;;
  esac
fi

# 2. Staging a secret file.
add_re="git${sp}+add${sp}+($sep[[:space:]/])?\.env($sp|\$)"
if [[ $lc =~ $add_re ]]; then
  deny "never stage .env. It is gitignored on purpose (RULES.md rule 5)."
fi

# 3. Bypassing hooks or rewriting shared history.
noverify_re="git${sp}+(commit|push|merge|rebase)$sep--no-verify"
if [[ $lc =~ $noverify_re ]]; then
  deny "do not bypass git hooks. Fix what the hook reports (RULES.md rule 10)."
fi
force_re="git${sp}+push$sep(${sp}--force(${sp}|\$)|${sp}-f(${sp}|\$))"
if [[ $lc =~ $force_re ]]; then
  deny "no force-push. If history needs fixing, ask the human (RULES.md rule 10)."
fi

exit 0
