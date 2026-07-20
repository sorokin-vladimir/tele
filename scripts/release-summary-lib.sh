#!/usr/bin/env bash
# Pure helpers for scripts/release-summary.sh: parse a captured GoReleaser log
# into per-target publish statuses and render the job-summary table.
# Sourced, not executed. No git or network side effects.

# summary_strip_ansi
# Filter: remove ANSI SGR color codes from stdin. A literal ESC byte is injected
# via printf so this works on both BSD sed (macOS) and GNU sed (Linux).
summary_strip_ansi() {
  sed "s/$(printf '\033')\[[0-9;]*m//g"
}

# summary_failed_pipes <logfile>
# Echo the canonical target keys GoReleaser reported as failed, one per line.
# Failures are read only from the trailing "error occurred:" block, which is
# authoritative: a pipe can run visually to completion yet be reported here.
# GoReleaser pipe id -> key: winget->winget, scoop->scoop, aur->aur,
# snapcraft->snap, release->github.
summary_failed_pipes() {
  summary_strip_ansi <"$1" | awk '
    /error occurred:/ { inblock = 1; next }
    inblock && match($0, /\* [a-z]+:/) {
      name = substr($0, RSTART + 2, RLENGTH - 3)
      if      (name == "winget")    print "winget"
      else if (name == "scoop")     print "scoop"
      else if (name == "aur")       print "aur"
      else if (name == "snapcraft") print "snap"
      else if (name == "release")   print "github"
    }
  '
}

# summary_status <logfile> <target>
# target is one of: github scoop winget aur snap
# Echo exactly one of: published | failed | skipped | not run
summary_status() {
  local logfile=$1 target=$2 failed
  failed=$(summary_failed_pipes "$logfile")
  # A failure in the aggregated error block wins over any earlier success line.
  if printf '%s\n' "$failed" | grep -qx "$target"; then
    echo "failed"
    return
  fi
  summary_strip_ansi <"$logfile" | awk -v target="$target" '
    function header(name) {
      if      (name == "scm releases")           { cur = "github"; return 1 }
      else if (name == "winget")                 { cur = "winget"; return 1 }
      else if (name == "arch user repositories") { cur = "aur";    return 1 }
      else if (name == "scoop manifests")        { cur = "scoop";  return 1 }
      else if (name == "snapcraft packages")     { cur = "snap";   return 1 }
      return 0
    }
    {
      line = $0
      t = line
      sub(/^ *• /, "", t)   # strip indent + bullet, if present
      sub(/ +$/, "", t)     # strip trailing spaces
      if (header(t)) next
      if (cur != target) next
      if (line ~ /pipe skipped or partially skipped/)                 status = "skipped"
      else if (target == "github" && line ~ /release published/)      status = "published"
      else if (target == "winget" && line ~ /pull request created/)   status = "published"
      else if ((target == "scoop" || target == "aur") && line ~ /pushing/) status = "published"
    }
    END { print (status == "" ? "not run" : status) }
  '
}

# summary_render <github> <scoop> <winget> <aur> <snap> <homebrew> <gemfury>
# Echo a GitHub-flavored markdown status table.
summary_render() {
  printf '| Target         | Status    |\n'
  printf '| -------------- | --------- |\n'
  printf '| GitHub release | %s |\n' "$1"
  printf '| scoop          | %s |\n' "$2"
  printf '| winget         | %s |\n' "$3"
  printf '| AUR            | %s |\n' "$4"
  printf '| snap           | %s |\n' "$5"
  printf '| Homebrew       | %s |\n' "$6"
  printf '| Gemfury        | %s |\n' "$7"
}

# summary_any_failed <github> <scoop> <winget> <aur> <snap> <homebrew> <gemfury> <goreleaser_outcome>
# Echo 1 if any target failed, or GoReleaser failed without naming a pipe
# (a pre-publish failure such as a build error); else 0.
summary_any_failed() {
  local goreleaser_outcome=${8:-}
  local s
  for s in "$1" "$2" "$3" "$4" "$5" "$6" "$7"; do
    [ "$s" = "failed" ] && {
      echo 1
      return
    }
  done
  [ "$goreleaser_outcome" = "failure" ] && {
    echo 1
    return
  }
  echo 0
}
