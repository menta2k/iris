#!/bin/sh
# preremove — runs before files are removed, on both deb (prerm) and rpm
# (%preun). Stop + disable the service so we don't leave a dangling unit.
set -e

if command -v systemctl >/dev/null 2>&1; then
    systemctl stop iris >/dev/null 2>&1 || true
    systemctl disable iris >/dev/null 2>&1 || true
fi

exit 0
