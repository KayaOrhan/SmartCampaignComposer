#!/usr/bin/env bash
# Redact secrets and personal data from a session export.
# Usage: scrub.sh <input-file> <output-file>
# Exit codes: 0 clean, 1 something still looks sensitive after scrubbing, 2 usage or tool error.
set -u

[ $# -eq 2 ] || { echo "usage: scrub.sh <input> <output>" >&2; exit 2; }
in=$1; out=$2
[ -f "$in" ] || { echo "no such file: $in" >&2; exit 2; }
[ "$in" != "$out" ] || { echo "output must differ from input; keep the raw file untouched" >&2; exit 2; }
command -v perl >/dev/null 2>&1 || { echo "perl is required" >&2; exit 2; }

# Each rule: name, regex, replacement sub. Specific keys come before generic ones.
perl -CSD -e '
  my $lit = sub { my $t = shift; sub { $t } };
  my @rules = (
    ["anthropic_key", qr/sk-ant-[A-Za-z0-9_\-]{8,}/,                $lit->("[REDACTED_ANTHROPIC_KEY]")],
    ["openai_style",  qr/\bsk-[A-Za-z0-9]{20,}/,                    $lit->("[REDACTED_API_KEY]")],
    ["github_token",  qr/\bgh[pousr]_[A-Za-z0-9]{20,}/,             $lit->("[REDACTED_GITHUB_TOKEN]")],
    ["aws_key",       qr/\bAKIA[0-9A-Z]{16}\b/,                     $lit->("[REDACTED_AWS_KEY]")],
    ["bearer",        qr/\b(Bearer)\s+[A-Za-z0-9._~+\/=\-]{16,}/,   sub { "$_[0] [REDACTED]" }],
    ["kv_secret",     qr/\b([A-Za-z0-9_]*(?:API_?KEY|SECRET|TOKEN|PASSWORD|PASSWD)[A-Za-z0-9_]*)(\s*[:=]\s*)(?!\[REDACTED)("[^"\n]+"|\x27[^\x27\n]+\x27|[^\s"\x27,;]+)/i,
                                                                    sub { $_[0] . $_[1] . "[REDACTED]" }],
    ["email",         qr/[A-Za-z0-9._%+\-]+@(?!example\.test\b)[A-Za-z0-9.\-]+\.[A-Za-z]{2,}/, $lit->("[REDACTED_EMAIL]")],
    ["tr_phone",      qr/(?<!\d)(?:\+?90[\s\-]?|0)5\d{2}[\s\-]?\d{3}[\s\-]?\d{2}[\s\-]?\d{2}(?!\d)/, $lit->("[REDACTED_PHONE]")],
    ["home_path",     qr{/(?:home|Users)/(?!USER\b)[A-Za-z0-9._\-]+}, $lit->("/home/USER")],
  );
  my %n;
  open(my $fh, "<:utf8", $ARGV[0]) or die "read: $!";
  open(my $oh, ">:utf8", $ARGV[1]) or die "write: $!";
  while (my $line = <$fh>) {
    for my $r (@rules) {
      my ($name, $re, $rep) = @$r;
      my $c = ($line =~ s/$re/$rep->($1 \/\/ "", $2 \/\/ "")/ge);
      $n{$name} += $c if $c;
    }
    print $oh $line;
  }
  close $oh;
  print STDERR "redactions:\n";
  print STDERR "  none\n" unless %n;
  printf STDERR "  %-14s %d\n", $_, $n{$_} for sort keys %n;
' "$in" "$out" || exit 2

# Second pass on the OUTPUT: strict patterns that must never survive.
left=0
tmp=$(mktemp)
check() { # name regex
  if grep -nE "$2" "$out" >"$tmp" 2>/dev/null; then
    echo "STILL PRESENT ($1):" >&2; sed 's/^/  line /' "$tmp" | cut -c1-120 >&2; left=1
  fi
}
check "anthropic key" 'sk-ant-[A-Za-z0-9_-]{8,}'
check "github token"  'gh[pousr]_[A-Za-z0-9]{20,}'
check "aws key"       'AKIA[0-9A-Z]{16}'
check "private key"   '-----BEGIN [A-Z ]*PRIVATE KEY-----'
rm -f "$tmp"

if [ $left -ne 0 ]; then echo "Scrub incomplete. Fix by hand, do not commit." >&2; exit 1; fi
echo "Scrubbed copy written to $out. Review it by eye before committing: no scrubber is perfect." >&2
exit 0
