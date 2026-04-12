# Agent playbook

This document explains how coding agents should use `devlane`.

## Default sequence

When asked to work inside a repo that uses `devlane`, agents should prefer this sequence:

1. `inspect --json`
2. `prepare`
3. `up` or `status`

That order keeps discovery explicit and reproducible.

## What agents should not do first

Avoid leading with:

- guessing ports
- editing generated env files directly
- scraping random shell scripts
- assuming stable and dev use the same naming rules

## If a repo has no adapter yet

Agents should:

1. read `docs/50-adapter-schema.md`
2. start from `examples/minimal-web/` or the closest example repo
3. write a small `devlane.yaml`
4. wire the repo's current generated files into `outputs.generated`
5. make `inspect --json` and `prepare` produce the current local state deterministically

## What to ask from a repo

A repo adopting `devlane` only needs to answer a small number of questions:

- what is the app identifier?
- is it `web`, `cli`, or `hybrid`?
- what does stable own?
- which files should be generated?
- which Compose files and profiles exist?
- if it is a web app, what hostname pattern should dev lanes use?

## Good agent behavior

Good agents leave behind:

- updated docs
- updated example adapters
- updated schemas
- deterministic generated outputs
- explicit acceptance notes

The goal is not only to implement the change, but to preserve the next agent's ability to reason about it quickly.
