#!/bin/sh
# Iris postinstall — runs after the package files land. Idempotent:
# on upgrade we re-run the same script and want it to be a no-op for
# steps that already happened on the first install.
#
# Refuses to start the service automatically. Operator must:
#   1. edit /etc/iris/iris.env (JWT secrets, kumomta paths)
#   2. systemctl enable --now iris
#
# This is the standard "needs config before first start" posture for
# server packages — auto-starting with placeholder secrets would let
# the binary refuse-to-boot loudly in journald, which is worse than
# being explicit about the install gate.
set -e

# 1. System user/group — the systemd unit specifies User=iris.
#    Matching pattern from existing OS packages (e.g. nginx, redis):
#    create as a system account with no shell, no home directory, no
#    expiry. `--system` skips the password prompt and uses the
#    system UID/GID range.
if ! getent group iris >/dev/null 2>&1; then
    groupadd --system iris
fi
if ! getent passwd iris >/dev/null 2>&1; then
    useradd --system \
            --gid iris \
            --shell /usr/sbin/nologin \
            --home-dir /var/lib/iris \
            --no-create-home \
            iris
fi

# 2. If kumomta's package installed a `kumomta` group (it does, on
#    a stock kumomta install), add iris to it so the service can
#    write the rendered policy + DKIM keys into shared dirs without
#    a world-writable mode. Tolerates the group not existing — iris
#    still works, it just can't render to /opt/kumomta/etc/ until
#    permissions are sorted manually.
if getent group kumomta >/dev/null 2>&1; then
    if ! id -nG iris | tr ' ' '\n' | grep -qx kumomta; then
        usermod -aG kumomta iris
    fi
fi

# 3. First-install only — copy iris.env.example to iris.env so the
#    operator has a starting point. We never overwrite an existing
#    iris.env (that's where they put their JWT secrets).
if [ ! -f /etc/iris/iris.env ]; then
    cp /etc/iris/iris.env.example /etc/iris/iris.env
fi

# 4. Permissions on the env file — root:iris 0640 keeps secrets
#    invisible to non-iris UIDs while letting the service read.
chown root:iris /etc/iris/iris.env  || true
chmod 0640 /etc/iris/iris.env       || true

# 5. /var/lib/iris owned by iris:iris (nfpm sets this at install but
#    chown is cheap and survives operator-driven recreates).
chown -R iris:iris /var/lib/iris    || true

# 6. systemd reload so a freshly-installed unit is visible to
#    `systemctl status iris`. We don't enable or start.
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload         || true
fi

cat <<'EOF'

iris is installed but NOT started. Next steps:

  1. Edit /etc/iris/iris.env and set:
     - IRIS_AUTH_ACCESS_SECRET   (openssl rand -base64 48)
     - IRIS_AUTH_REFRESH_SECRET  (openssl rand -base64 48)
     - IRIS_KUMO_API_ENDPOINT    (e.g. http://127.0.0.1:8025)
     - IRIS_LOGSTREAM_REDIS_URL  (e.g. redis://127.0.0.1:6379/0)

  2. Confirm /etc/iris/configs/data.yaml points at your Postgres
     (and that the database has the timescaledb extension).

  3. systemctl enable --now iris

Full walkthrough: /usr/share/doc/iris/host-native-deploy.md or
https://github.com/menta2k/iris/blob/main/docs/host-native-deploy.md
EOF

exit 0
