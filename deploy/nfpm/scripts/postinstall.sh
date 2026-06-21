#!/bin/sh
# postinstall — runs after files are unpacked, on both deb (postinst) and
# rpm (%post). Idempotent: safe on fresh install and upgrade.
set -e

# Create the dedicated system user the unit runs as.
if ! getent passwd iris >/dev/null 2>&1; then
    useradd --system --no-create-home --home-dir /var/lib/iris \
        --shell /usr/sbin/nologin iris 2>/dev/null \
        || adduser --system --home /var/lib/iris --no-create-home iris 2>/dev/null \
        || true
fi

# Seed the env file from the template on first install only; never clobber an
# operator's existing secrets on upgrade.
if [ ! -f /etc/iris/iris.env ]; then
    cp /etc/iris/iris.env.example /etc/iris/iris.env
    chmod 0640 /etc/iris/iris.env
fi

# Lock down the secret-bearing files to the iris group.
chown -R root:iris /etc/iris 2>/dev/null || true
chmod 0750 /etc/iris 2>/dev/null || true
chmod 0640 /etc/iris/iris.yaml 2>/dev/null || true
chown -R iris:iris /var/lib/iris 2>/dev/null || true

# Reload systemd so it sees the new unit. Don't auto-start — the operator must
# fill in /etc/iris/iris.env first.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

echo "iris installed. Edit /etc/iris/iris.env (secrets) and /etc/iris/iris.yaml,"
echo "then: systemctl enable --now iris"

exit 0
