#!/bin/sh
# Iris preremove — runs before the package files are deleted.
#
# Stop + disable the service so we don't leave a dangling unit
# pointing at a binary that's about to vanish. We do NOT remove the
# iris user or /etc/iris/iris.env here — that's postremove's job and
# only on full purge, not upgrade. nfpm passes "$1 = upgrade" on
# upgrades; we tolerate that gracefully and skip the stop entirely.
set -e

# nfpm forwards the lifecycle action via $1 on rpm-derived flows.
# On Debian, dpkg passes "upgrade" or "remove". Either way: skip
# the stop on upgrade so the service stays running across the
# brief moment the new files are dropped in.
case "${1:-}" in
    upgrade|0|1)
        # rpm: $1=1 means upgrade (one package will remain after this
        # transaction). $1=0 means full removal. dpkg: literal
        # "upgrade". Either form, leave the service alone — postinst
        # of the new package will reload + (the operator) restart.
        exit 0
        ;;
esac

if [ -d /run/systemd/system ]; then
    # --no-reload keeps a redundant daemon-reload off the stop path;
    # we'll reload in postremove anyway.
    systemctl --no-reload disable --now iris.service || true
fi

exit 0
