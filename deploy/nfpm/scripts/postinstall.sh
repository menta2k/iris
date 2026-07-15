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

# Cluster mTLS material is created at enrollment (not shipped by the package)
# and is read by processes that run as the iris user: on a node, iris-agent
# reads /etc/iris/cluster/agent.key; on the control plane, iris reads its client
# key and the CA key under /etc/iris/cluster and /etc/iris/cluster-ca. Those
# private keys are 0600, so the blanket "chown -R root:iris" above locks the
# iris user out ("load agent certificate: permission denied") — and because
# this script re-runs on every upgrade, a working node breaks after each
# `dpkg`/`apt` update. Re-assert iris ownership on the cluster dirs so the keys
# stay readable across upgrades.
for d in /etc/iris/cluster /etc/iris/cluster-ca; do
    if [ -d "$d" ]; then
        chown -R iris:iris "$d" 2>/dev/null || true
    fi
done

# Grant kumod read access to the generated KumoMTA policy.
#
# Iris writes /opt/kumomta/etc/policy/iris_generated.lua as iris:iris 0640 (it
# holds secrets, so it is not world-readable). kumomta starts as root and drops
# to the kumod *user* keeping only its primary group, so adding kumod to the
# iris group does NOT help — the daemon never has it. A default ACL granting the
# kumod user read makes every file Iris writes there (temp-file + rename)
# readable, and survives regeneration. Best-effort: skipped silently when the
# dir doesn't exist (kumomta not installed yet), setfacl is unavailable, or the
# filesystem lacks ACL support. NOTE: keyed to the default config_path; if you
# set kumomta.config_path elsewhere, apply the same ACL to that directory.
POLICY_DIR=/opt/kumomta/etc/policy
if [ -d "$POLICY_DIR" ] && getent passwd kumod >/dev/null 2>&1; then
    if command -v setfacl >/dev/null 2>&1; then
        setfacl -m u:kumod:rx "$POLICY_DIR" 2>/dev/null || true
        setfacl -m d:u:kumod:rx "$POLICY_DIR" 2>/dev/null || true
        if [ -f "$POLICY_DIR/iris_generated.lua" ]; then
            setfacl -m u:kumod:r "$POLICY_DIR/iris_generated.lua" 2>/dev/null || true
        fi
    else
        echo "iris: 'setfacl' not found — install the 'acl' package, then run:" >&2
        echo "  setfacl -m d:u:kumod:rx $POLICY_DIR" >&2
        echo "so kumod can read the generated policy." >&2
    fi
fi

# Reload systemd so it sees the new unit. Don't auto-start — the operator must
# fill in /etc/iris/iris.env first.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

echo "iris installed. Edit /etc/iris/iris.env (secrets) and /etc/iris/iris.yaml,"
echo "then: systemctl enable --now iris"

exit 0
