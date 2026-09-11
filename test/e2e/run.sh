#!/bin/sh
# The end-to-end suite under one throwaway directory. HOME points into it so
# the machine's own monitor registry stays out of the fixtures, and every
# fixture a script would otherwise leave under /tmp is placed inside it: the
# directory is removed on exit, pass or fail, and a monitor that a fixture's
# `run start` spawned stops within seconds of its authority disappearing. Run
# against the real HOME the fixtures once left four hundred temporary
# authorities registered and a monitor re-reading them for days; left under
# /tmp, one day of gates left ninety fixtures and a dev-build monitor holding
# the port a real authority's monitor needed.
set -eu
binary="$1"
home=$(mktemp -d /tmp/prifly-e2e-XXXXXX)
trap 'rm -rf "$home"' EXIT
export HOME="$home"
export XDG_CONFIG_HOME="$home/.config"
sh test/e2e/verify-install.sh
python3 -B test/e2e/test_examples.py
python3 -B test/e2e/verify-authoring.py --binary "$binary"
python3 -B test/e2e/verify-cli.py --binary "$binary" --target "$home/fixtures/cli"
python3 -B test/e2e/verify-core.py --binary "$binary" --target "$home/fixtures/core"
python3 -B test/e2e/verify-context.py --binary "$binary" --target "$home/fixtures/context"
python3 -B test/e2e/verify-parallel-worktrees.py --binary "$binary" --target "$home/fixtures/parallel"
python3 -B test/e2e/verify-monitor.py --binary "$binary" --target "$home/fixtures/monitor"
python3 -B test/e2e/verify-capacity.py --binary "$binary" --target "$home/fixtures/capacity"
