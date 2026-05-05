#!/bin/sh
# Iris postremove — runs after the package files are gone.
#
# Cleans up:
#   - reloads systemd so the now-missing unit doesn't linger as
#     "not-found" in `systemctl list-unit-files`
#   - on full purge (not upgrade), removes the iris user
#
# What we deliberately DO NOT remove:
#   - /etc/iris/iris.env   (operator's secrets, may be needed for re-install)
#   - /etc/iris/configs/   (operator may have customised)
#   - /var/lib/iris/       (operator state)
#   - the kumomta group membership (if iris was added to it)
# Removing config + state on uninstall is hostile; operators can rm -rf
# manually if they truly want to.
set -e

# Don't drop the user on upgrade — the new package needs it.
# (Same lifecycle-arg conventions as preremove.)
case "${1:-}" in
    upgrade|1)
        exit 0
        ;;
esac

# systemd reload — drops the disappeared unit from the catalog.
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
fi

# Full removal: drop the iris user/group. We don't touch
# /var/lib/iris ownership — it'll be 65534:65534 (nobody) afterward
# but the operator can chown if they re-install.
if getent passwd iris >/dev/null 2>&1; then
    userdel iris  || true
fi
if getent group iris >/dev/null 2>&1; then
    groupdel iris || true
fi

exit 0
