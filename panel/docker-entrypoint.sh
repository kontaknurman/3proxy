#!/bin/sh
# Runs 3proxy next to the panel in one container and restarts it when it
# exits. The panel writes the configuration and reloads 3proxy with SIGUSR1
# through PANEL_RELOAD.
cfg="${PANEL_CONFIG:-/data/3proxy.cfg}"

(
	# The panel writes the configuration when it starts.
	while [ ! -f "$cfg" ]; do sleep 1; done
	while :; do
		3proxy "$cfg"
		echo "3proxy exited with status $?, restarting in 2 seconds" >&2
		sleep 2
	done
) &

exec 3proxy-panel "$@"
