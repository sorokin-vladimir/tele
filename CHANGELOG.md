# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

A human title for a release is written as an em-dash suffix on its heading,
e.g. `## [1.2.0] - 2026-06-11 — Archived folders & image layout fixes`.
Older releases are at https://github.com/sorokin-vladimir/tele/releases.

## [Unreleased] — Reliable updates after long idle
### Fixed
- Messages and updates keep arriving after the app has been idle for a long
  time, instead of silently stalling until restart (#119)
