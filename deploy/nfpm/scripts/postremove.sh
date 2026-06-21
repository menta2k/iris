#!/bin/sh
# postremove — runs after files are removed, on both deb (postrm) and rpm
# (%postun). Reload systemd so the removed unit is forgotten. We deliberately
# do NOT delete /etc/iris (operator config) or /var/lib/iris (state) or the
# iris user, so a reinstall keeps working and a purge is an explicit choice.
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

exit 0
