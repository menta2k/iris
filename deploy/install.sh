#!/usr/bin/env bash
# install.sh — bootstrap a stand-alone host install of Iris.
#
# Brings up everything the admin-service depends on, on a single
# Linux host, in this order:
#
#   1. Valkey       (Redis-compatible; log-stream + suppression cache)
#   2. PostgreSQL   (PGDG repo for the version TimescaleDB targets)
#   3. TimescaleDB  (Postgres extension; primary storage)
#   4. Prometheus   (scrapes /metrics on 127.0.0.1:9090)
#   5. KumoMTA      (the actual mail server; iris is its operator UI)
#   6. Iris         (deb/rpm from the GitHub release)
#
# KumoMTA is installed before Iris so the iris .deb/.rpm postinst
# finds the `kumomta` group and auto-joins the iris user to it
# (iris writes /opt/kumomta/etc/policy/init.lua, which kumomta reads).
# The package's default init.lua is enough to get the daemon running;
# you point iris at kumomta and use the UI's "Apply policy" to write
# a real configuration.
#
# Usage (root or via sudo):
#
#   curl -fsSL https://raw.githubusercontent.com/menta2k/iris/main/deploy/install.sh \
#       | sudo bash
#
# Or with knobs:
#
#   sudo IRIS_VERSION=v0.1.0 PG_MAJOR=16 ./install.sh
#
# Idempotent: running twice does no harm. Existing JWT secrets in
# /etc/iris/iris.env are preserved; the script only fills empty ones.

set -euo pipefail

# ──────────────────────────────────────────────────────────────────────
# Knobs (override via env). Pin defaults so re-runs converge on the
# same versions; bump explicitly when upgrading.
# ──────────────────────────────────────────────────────────────────────
IRIS_VERSION="${IRIS_VERSION:-v0.1.0}"
IRIS_REPO="${IRIS_REPO:-menta2k/iris}"
PG_MAJOR="${PG_MAJOR:-16}"
PROMETHEUS_VERSION="${PROMETHEUS_VERSION:-2.55.1}"
PROMETHEUS_USER="${PROMETHEUS_USER:-prometheus}"
PROMETHEUS_DATA_DIR="${PROMETHEUS_DATA_DIR:-/var/lib/prometheus}"
PROMETHEUS_CONFIG_DIR="${PROMETHEUS_CONFIG_DIR:-/etc/prometheus}"

# Iris service wiring — defaults match the systemd unit shipped in the
# package. Override only if you know why.
IRIS_PG_USER="${IRIS_PG_USER:-iris}"
IRIS_PG_DB="${IRIS_PG_DB:-iris}"
IRIS_REDIS_URL="${IRIS_REDIS_URL:-redis://127.0.0.1:6379/0}"

# Initial admin account. Iris's auto-migrate seeds `admin/admin` from
# 0003_seed_admin.sql. Set IRIS_ADMIN_PASSWORD to override (the script
# bcrypt-hashes it with cost 12 to match auth.yaml). Empty = keep the
# default and warn the operator to rotate.
IRIS_ADMIN_USERNAME="${IRIS_ADMIN_USERNAME:-admin}"
IRIS_ADMIN_PASSWORD="${IRIS_ADMIN_PASSWORD:-}"

# ──────────────────────────────────────────────────────────────────────
# Output helpers — keep stderr/stdout tidy for `| tee install.log`.
# ──────────────────────────────────────────────────────────────────────
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n'  "$*" >&2; }
fail() { printf '\033[1;31mXX\033[0m %s\n'  "$*" >&2; exit 1; }

# ──────────────────────────────────────────────────────────────────────
# Preconditions
# ──────────────────────────────────────────────────────────────────────
[[ $EUID -eq 0 ]] || fail "Run as root (sudo)."

ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) DEB_ARCH=amd64; RPM_ARCH=x86_64 ;;
    *) fail "Unsupported architecture: $ARCH (only x86_64 packages are published for $IRIS_VERSION)." ;;
esac

# ──────────────────────────────────────────────────────────────────────
# OS detection — sets OS_FAMILY (apt|dnf), OS_ID, OS_CODENAME.
# Anything we can't classify exits early; better than installing
# half a stack and failing on a missing package manager later.
# ──────────────────────────────────────────────────────────────────────
detect_os() {
    [[ -f /etc/os-release ]] || fail "/etc/os-release missing — unsupported OS."
    # shellcheck disable=SC1091
    . /etc/os-release
    OS_ID="${ID:-}"
    OS_VERSION_ID="${VERSION_ID:-}"
    OS_CODENAME="${VERSION_CODENAME:-${UBUNTU_CODENAME:-}}"
    OS_LIKE="${ID_LIKE:-}"

    case "$OS_ID $OS_LIKE" in
        *debian*|*ubuntu*) OS_FAMILY=apt ;;
        *rhel*|*fedora*|*centos*|*rocky*|*almalinux*|*ol*) OS_FAMILY=dnf ;;
        *)
            case "$OS_ID" in
                debian|ubuntu) OS_FAMILY=apt ;;
                rhel|centos|fedora|rocky|almalinux|ol|oracle) OS_FAMILY=dnf ;;
                *) fail "Unsupported OS: $OS_ID. This installer supports Debian/Ubuntu and RHEL/Rocky/Fedora." ;;
            esac
            ;;
    esac

    # dnf is the modern package manager; yum is the older one. They take
    # the same arguments for `install`, but on EL7 only yum exists.
    if [[ "$OS_FAMILY" == "dnf" ]]; then
        if command -v dnf >/dev/null; then PKG=dnf; else PKG=yum; fi
    else
        PKG=apt-get
        export DEBIAN_FRONTEND=noninteractive
    fi

    log "Detected: $OS_ID $OS_VERSION_ID ($OS_FAMILY family, $PKG)"
}

# ──────────────────────────────────────────────────────────────────────
# Step 1 — Valkey
#
# Per https://valkey.io/topics/installation/. The package name is
# `valkey` everywhere; the redis-compat shim (`valkey-redis-compat`
# on apt, `valkey-compat-redis` on dnf) exposes a `redis.service`
# alias for upstream tools that hard-code the redis name.
# ──────────────────────────────────────────────────────────────────────
install_valkey() {
    # Pre-flight: a running redis-server holds :6379 and prevents the
    # newly-installed valkey-server from starting. valkey-redis-compat
    # is meant to *replace* redis-server, but apt won't auto-stop the
    # existing service. Stop+disable+mask in that order so the unit
    # can't come back via socket activation.
    if systemctl list-unit-files redis-server.service >/dev/null 2>&1; then
        if systemctl is-active --quiet redis-server.service 2>/dev/null; then
            log "[1/6] Stopping conflicting redis-server (port 6379)…"
            systemctl stop redis-server.service || true
        fi
        systemctl disable redis-server.service 2>/dev/null || true
        systemctl mask    redis-server.service 2>/dev/null || true
    fi
    if systemctl list-unit-files redis.service >/dev/null 2>&1; then
        systemctl is-active --quiet redis.service 2>/dev/null && systemctl stop redis.service || true
        systemctl disable redis.service 2>/dev/null || true
    fi

    if command -v valkey-cli >/dev/null 2>&1; then
        log "[1/6] Valkey already installed — skipping package install."
    else
        log "[1/6] Installing Valkey…"
        if [[ "$OS_FAMILY" == "apt" ]]; then
            apt-get update -y
            apt-get install -y valkey valkey-redis-compat
        else
            # EL needs EPEL for valkey on most major versions.
            $PKG install -y epel-release || true
            $PKG install -y valkey valkey-compat-redis || $PKG install -y valkey
        fi
    fi

    # Service name varies: debian → valkey-server.service,
    # fedora → valkey.service. Try both.
    for svc in valkey-server valkey; do
        if systemctl list-unit-files "${svc}.service" >/dev/null 2>&1; then
            VALKEY_SERVICE="${svc}.service"
            break
        fi
    done
    [[ -n "${VALKEY_SERVICE:-}" ]] || fail "Valkey installed but no systemd unit found (looked for valkey-server, valkey)."

    # Postinst-fix: on Ubuntu Noble (24.04) the valkey package's
    # postinst doesn't always chown /var/lib/valkey to the valkey user,
    # leaving the daemon unable to write its dump file. Belt-and-braces.
    if id -u valkey >/dev/null 2>&1; then
        for d in /var/lib/valkey /var/log/valkey /run/valkey; do
            if [[ -d "$d" ]]; then
                chown -R valkey:valkey "$d" 2>/dev/null || true
            fi
        done
    fi

    # Locale fix: Valkey 7.2 calls setlocale(LC_COLLATE, "") at boot,
    # which fails fatally with `Failed to configure LOCALE for invalid
    # locale name` on hosts where LC_*/LANG are unset or point at a
    # locale that wasn't generated (cloud images often ship with no
    # locale-gen'd UTF-8 locale). Pin the unit to C.UTF-8 — it's
    # built-in, always available, doesn't require locale-gen, and
    # doesn't change anything system-wide.
    mkdir -p "/etc/systemd/system/${VALKEY_SERVICE}.d"
    cat > "/etc/systemd/system/${VALKEY_SERVICE}.d/locale.conf" <<'EOF'
# Drop-in installed by deploy/install.sh — pins the unit's locale to
# C.UTF-8 to avoid the "Failed to configure LOCALE for invalid locale
# name" startup crash on hosts without a generated UTF-8 locale.
[Service]
Environment=LC_ALL=C.UTF-8
Environment=LANG=C.UTF-8
EOF

    # If a previous install attempt looped enough to trip systemd's
    # start-rate limiter, the unit is now stuck in failed state until
    # we reset.
    systemctl reset-failed "$VALKEY_SERVICE" 2>/dev/null || true
    # Daemon-reload covers the redis mask above; otherwise systemd may
    # still consider redis the running service-of-record for :6379.
    systemctl daemon-reload
    if ! systemctl enable --now "$VALKEY_SERVICE"; then
        warn "$VALKEY_SERVICE failed to start. Recent journal:"
        journalctl -xeu "$VALKEY_SERVICE" --no-pager -n 40 >&2 || true
        fail "$VALKEY_SERVICE did not start cleanly. Common causes: port 6379 already in use, /var/lib/valkey ownership, or a syntax error in /etc/valkey/valkey.conf."
    fi
    log "[1/6] Valkey running ($VALKEY_SERVICE)."
}

# ──────────────────────────────────────────────────────────────────────
# Step 2 — TimescaleDB (which pulls PostgreSQL via PGDG repo).
#
# Per https://www.tigerdata.com/docs/get-started/choose-your-path/install-timescaledb.
# We add the PGDG repo first so we get the same Postgres major that
# the TimescaleDB packages target ($PG_MAJOR), then add the Timescale
# packagecloud repo and install both.
# ──────────────────────────────────────────────────────────────────────
install_timescaledb() {
    if pg_isready -q 2>/dev/null && psql -V 2>/dev/null | grep -q "PostgreSQL.*${PG_MAJOR}\."; then
        # Already have the right Postgres major. Check for the extension.
        if su -l postgres -c "psql -tAc \"SELECT 1 FROM pg_available_extensions WHERE name='timescaledb'\"" 2>/dev/null | grep -q 1; then
            log "[2/6] PostgreSQL $PG_MAJOR + TimescaleDB already installed — skipping."
            return
        fi
    fi

    log "[2/6] Installing PostgreSQL $PG_MAJOR + TimescaleDB…"

    # Locale fix — postgresql-common's pg_createcluster reads $LANG
    # from the environment and aborts with "could not create default
    # cluster" if that locale wasn't `locale-gen`'d. Cloud / minimal
    # Ubuntu images ship with LANG=en_US.UTF-8 set in /etc/default/locale
    # but no actual UTF-8 locale generated, so the install bombs and
    # timescaledb-tune then fails because postgresql.conf doesn't exist
    # (no cluster). Install `locales` and generate the host's stated
    # locale before any postgresql package can run its postinst.
    if [[ "$OS_FAMILY" == "apt" ]]; then
        local sys_lang
        sys_lang="${LANG:-en_US.UTF-8}"
        if ! locale -a 2>/dev/null | grep -qiE "^${sys_lang/-/}$|^${sys_lang/UTF-8/utf8}$"; then
            log "[2/6] Generating locale ${sys_lang} (postgresql-common requires it)…"
            apt-get install -y locales
            # locale-gen accepts the canonical "en_US.UTF-8" form; no
            # need to translate to the underscored variant.
            locale-gen "$sys_lang" || locale-gen en_US.UTF-8
            update-locale LANG="$sys_lang" || update-locale LANG=en_US.UTF-8
        fi
    fi

    if [[ "$OS_FAMILY" == "apt" ]]; then
        # Pre-clean a stale Timescale repo file BEFORE the next
        # apt-get update runs. apt.postgresql.org.sh -y kicks off its
        # own internal `apt-get update`; if a previous install run
        # left /etc/apt/sources.list.d/timescaledb.list pointing at an
        # unsupported codename (e.g. `noble`), that update returns
        # non-zero, set -e aborts, and we never reach the rewrite
        # step below. Nuking it here lets the PGDG setup succeed; we
        # write the correct repo file a few lines down.
        rm -f /etc/apt/sources.list.d/timescaledb.list

        apt-get install -y gnupg postgresql-common apt-transport-https lsb-release wget curl
        # apt.postgresql.org.sh is interactive by default; -y skips the
        # "do you want to add the PGDG repo" prompt.
        /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y

        # Timescale packagecloud repo + signing key.
        #
        # Timescale's packagecloud only publishes Debian codenames
        # (currently `bookworm` and `trixie`); no Ubuntu codenames are
        # supported. The Timescale docs still tell you to use
        # `$(lsb_release -c -s)`, which is broken on every Ubuntu
        # release. The packages themselves link against the
        # PostgreSQL ABI rather than Ubuntu-specific symbols, so
        # installing the bookworm timescale package on top of PGDG's
        # postgres-N binary on any modern Ubuntu is the documented
        # community workaround.
        #
        # Hardcoded mapping (over a network probe) because: the list
        # changes ~once a year, every probe alternative I tried had a
        # subtle exit-code quirk with packagecloud's CDN, and a wrong
        # answer here breaks `apt-get update` for the whole stack.
        local current_codename ts_codename
        current_codename=$(lsb_release -c -s)
        case "$OS_ID:$current_codename" in
            debian:bookworm|debian:trixie)
                # Native — host codename is on the supported list.
                ts_codename="$current_codename"
                ;;
            *)
                # Everything else (Ubuntu, derivatives, older Debian)
                # uses bookworm packages. Never trixie as a fallback —
                # it was added recently and not every PG major has a
                # build there yet.
                ts_codename=bookworm
                warn "Timescale doesn't publish for ${OS_ID} ${current_codename}; using bookworm packages (ABI-compatible)."
                ;;
        esac

        # Force-rewrite if the file is missing OR points at a different
        # codename. A previous run with stale logic may have written
        # `noble` (or whatever lsb_release returned), which apt then
        # 404s on every subsequent `update`.
        if [[ ! -f /etc/apt/sources.list.d/timescaledb.list ]] || \
           ! grep -q " ${ts_codename} " /etc/apt/sources.list.d/timescaledb.list; then
            echo "deb https://packagecloud.io/timescale/timescaledb/debian/ ${ts_codename} main" \
                > /etc/apt/sources.list.d/timescaledb.list
            # `gpg --dearmor` prompts on overwrite; remove the existing
            # keyring first so re-runs don't hang waiting for input.
            rm -f /etc/apt/trusted.gpg.d/timescaledb.gpg
            wget --quiet -O - https://packagecloud.io/timescale/timescaledb/gpgkey \
                | gpg --dearmor -o /etc/apt/trusted.gpg.d/timescaledb.gpg
        fi
        apt-get update -y
        # postgresql-${PG_MAJOR} is what actually creates the cluster.
        # The previous version of this script only pulled in
        # timescaledb-2-postgresql-${PG_MAJOR}, which depends on
        # postgresql-${PG_MAJOR} but doesn't re-trigger its postinst
        # if a previous install already half-succeeded. Pull it
        # explicitly so apt's dependency resolution actually runs the
        # cluster setup.
        apt-get install -y \
            "postgresql-${PG_MAJOR}" \
            "postgresql-client-${PG_MAJOR}" \
            "timescaledb-2-postgresql-${PG_MAJOR}"

        # If a previous run failed mid-postinst (typically the
        # locale issue handled above), the cluster directory may not
        # exist. pg_createcluster is the official way to bring it up
        # after the package is already installed; --start brings it
        # online in the same call.
        if [[ ! -f "/etc/postgresql/${PG_MAJOR}/main/postgresql.conf" ]]; then
            log "[2/6] Creating PostgreSQL cluster (postinst skipped it)…"
            pg_createcluster "${PG_MAJOR}" main --start
        fi

        # tune writes shared_preload_libraries=timescaledb into postgresql.conf.
        timescaledb-tune --quiet --yes || timescaledb-tune --yes
        systemctl restart "postgresql@${PG_MAJOR}-main.service" 2>/dev/null \
            || systemctl restart postgresql
    else
        # PGDG repo for EL — provides postgresqlNN. The version_id parsing
        # handles "9.0", "8", etc. (Rocky/RHEL major).
        EL_MAJOR=$(rpm -E '%{rhel}')
        $PKG install -y "https://download.postgresql.org/pub/repos/yum/reporpms/EL-${EL_MAJOR}-x86_64/pgdg-redhat-repo-latest.noarch.rpm" || true
        # Disable the built-in postgresql module on EL8+ so the PGDG
        # version is what actually gets installed.
        $PKG -qy module disable postgresql 2>/dev/null || true

        # Timescale packagecloud repo. Use a heredoc so $EL_MAJOR and
        # $basearch are correctly evaluated (the latter at yum-runtime).
        cat > /etc/yum.repos.d/timescale_timescaledb.repo <<EOF
[timescale_timescaledb]
name=timescale_timescaledb
baseurl=https://packagecloud.io/timescale/timescaledb/el/${EL_MAJOR}/\$basearch
repo_gpgcheck=1
gpgcheck=0
enabled=1
gpgkey=https://packagecloud.io/timescale/timescaledb/gpgkey
sslverify=1
sslcacert=/etc/pki/tls/certs/ca-bundle.crt
metadata_expire=300
EOF
        $PKG install -y "timescaledb-2-postgresql-${PG_MAJOR}" "postgresql${PG_MAJOR}" "postgresql${PG_MAJOR}-server"

        # Initialise the cluster on first install (idempotent — exits
        # cleanly if already done).
        if [[ ! -d "/var/lib/pgsql/${PG_MAJOR}/data/base" ]]; then
            "/usr/pgsql-${PG_MAJOR}/bin/postgresql-${PG_MAJOR}-setup" initdb
        fi
        systemctl enable --now "postgresql-${PG_MAJOR}.service"
        timescaledb-tune --pg-config="/usr/pgsql-${PG_MAJOR}/bin/pg_config" --quiet --yes
        systemctl restart "postgresql-${PG_MAJOR}.service"
    fi

    log "[2/6] PostgreSQL $PG_MAJOR + TimescaleDB ready."
}

# ──────────────────────────────────────────────────────────────────────
# Step 2b — provision the iris role + database.
#
# Runs as the `postgres` superuser. CREATE-IF-NOT-EXISTS is faked via
# pg_roles / pg_database lookups since plain CREATE doesn't support it.
# Idempotent: re-runs leave existing roles/DBs alone.
# ──────────────────────────────────────────────────────────────────────
init_iris_database() {
    log "[2/6] Provisioning iris role + database…"

    # Generate a random password on first install. Reuse the existing
    # one on subsequent runs (parsed back out of data.yaml so the
    # service keeps working).
    local pw_file=/etc/iris/.pgpassword
    if [[ -s "$pw_file" ]]; then
        IRIS_PG_PASSWORD=$(< "$pw_file")
    else
        IRIS_PG_PASSWORD=$(openssl rand -base64 24 | tr -d '/+=' | cut -c1-32)
        mkdir -p /etc/iris
        # 0600 root-only — only used by this script + iris's data.yaml.
        umask 077
        printf '%s' "$IRIS_PG_PASSWORD" > "$pw_file"
        umask 022
    fi

    # Create the role + db only if they don't already exist. Quote the
    # password literal carefully — single-quotes in SQL strings are
    # escaped by doubling.
    local pw_sql=${IRIS_PG_PASSWORD//\'/\'\'}
    su -l postgres -c "psql -tAc \"SELECT 1 FROM pg_roles WHERE rolname='${IRIS_PG_USER}'\"" \
        | grep -q 1 \
        || su -l postgres -c "psql -c \"CREATE ROLE ${IRIS_PG_USER} LOGIN PASSWORD '${pw_sql}'\""

    su -l postgres -c "psql -tAc \"SELECT 1 FROM pg_database WHERE datname='${IRIS_PG_DB}'\"" \
        | grep -q 1 \
        || su -l postgres -c "psql -c \"CREATE DATABASE ${IRIS_PG_DB} OWNER ${IRIS_PG_USER}\""

    # CREATE EXTENSION is per-database. Idempotent IF NOT EXISTS.
    su -l postgres -c "psql -d ${IRIS_PG_DB} -c \"CREATE EXTENSION IF NOT EXISTS timescaledb\""

    log "[2/6] Database '${IRIS_PG_DB}' owned by '${IRIS_PG_USER}' with timescaledb extension."
}

# ──────────────────────────────────────────────────────────────────────
# Step 3 — Prometheus (binary install + systemd unit + scrape config).
#
# No package-manager path that works cleanly across distros, so we
# pull the official release tarball, install the binaries to
# /usr/local/bin, and lay down a minimal scrape config that points
# at iris's metrics listener (default: 127.0.0.1:9090).
#
# Iris's metrics listener also defaults to 127.0.0.1:9090, which
# would collide with prometheus-server. We move iris to 9091 in the
# env file (see configure_iris_env) so both run on loopback without
# port conflict.
# ──────────────────────────────────────────────────────────────────────
install_prometheus() {
    if [[ -x /usr/local/bin/prometheus ]] && /usr/local/bin/prometheus --version 2>&1 | grep -q "$PROMETHEUS_VERSION"; then
        log "[3/6] Prometheus $PROMETHEUS_VERSION already installed — skipping binary."
    else
        log "[3/6] Installing Prometheus $PROMETHEUS_VERSION…"
        local tmp
        tmp=$(mktemp -d)
        # Cleanup tmp dir even if the script aborts mid-install.
        trap 'rm -rf "$tmp"' RETURN
        local url="https://github.com/prometheus/prometheus/releases/download/v${PROMETHEUS_VERSION}/prometheus-${PROMETHEUS_VERSION}.linux-${DEB_ARCH}.tar.gz"
        curl -fsSL -o "$tmp/prom.tgz" "$url"
        tar -xzf "$tmp/prom.tgz" -C "$tmp"
        local extracted="$tmp/prometheus-${PROMETHEUS_VERSION}.linux-${DEB_ARCH}"
        install -m 0755 "$extracted/prometheus" /usr/local/bin/prometheus
        install -m 0755 "$extracted/promtool"   /usr/local/bin/promtool
    fi

    # System user — owns the data dir but not the config.
    if ! id -u "$PROMETHEUS_USER" >/dev/null 2>&1; then
        useradd --system --no-create-home --shell /usr/sbin/nologin "$PROMETHEUS_USER"
    fi

    mkdir -p "$PROMETHEUS_CONFIG_DIR" "$PROMETHEUS_DATA_DIR"
    chown -R "$PROMETHEUS_USER:$PROMETHEUS_USER" "$PROMETHEUS_DATA_DIR"

    # Scrape config — only one target that matters: iris on 127.0.0.1:9091.
    # The 9090 self-scrape is by convention; useful when debugging the
    # scraper itself.
    if [[ ! -f "$PROMETHEUS_CONFIG_DIR/prometheus.yml" ]]; then
        cat > "$PROMETHEUS_CONFIG_DIR/prometheus.yml" <<'EOF'
# Stand-alone iris install — Prometheus scrape config.
#
# 15s strikes a balance between resolution and storage; the iris
# log-stream consumer's pendingGaugeLoop refreshes its gauge every
# 5s, so 15s gives us 3 samples per update window.

global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    env: standalone
    stack: iris

scrape_configs:
  # Iris admin-service — bound to loopback by the install script
  # to avoid colliding with prometheus-server on :9090.
  - job_name: iris
    static_configs:
      - targets: ['127.0.0.1:9091']
        labels:
          service: admin-service

  # Self-scrape so the local UI shows liveness for the scraper.
  - job_name: prometheus
    static_configs:
      - targets: ['127.0.0.1:9090']
EOF
    fi

    # Systemd unit — written to /etc/systemd/system so package upgrades
    # don't clobber it. Storage retention defaults to 15 days; bump
    # via --storage.tsdb.retention.time= in the unit if you need more.
    cat > /etc/systemd/system/prometheus.service <<EOF
[Unit]
Description=Prometheus monitoring server (iris stand-alone install)
Documentation=https://prometheus.io/docs/
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$PROMETHEUS_USER
Group=$PROMETHEUS_USER
ExecStart=/usr/local/bin/prometheus \\
    --config.file=$PROMETHEUS_CONFIG_DIR/prometheus.yml \\
    --storage.tsdb.path=$PROMETHEUS_DATA_DIR \\
    --web.console.templates=$PROMETHEUS_CONFIG_DIR/consoles \\
    --web.console.libraries=$PROMETHEUS_CONFIG_DIR/console_libraries \\
    --web.listen-address=0.0.0.0:9090 \\
    --storage.tsdb.retention.time=15d
Restart=on-failure
RestartSec=5s

# Hardening — Prometheus only needs read on the config dir and
# read-write on the data dir.
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$PROMETHEUS_DATA_DIR
PrivateTmp=true
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable --now prometheus
    log "[3/6] Prometheus running on :9090 (scraping iris on :9091)."
}

# ──────────────────────────────────────────────────────────────────────
# Step 4 — KumoMTA.
#
# Per https://docs.kumomta.com/userguide/installation/linux/. Two repo
# families: openrepo.kumomta.com on apt (Ubuntu 20/22), the same
# upstream on yum/dnf (Rocky 8/9, Amazon 2023). The packages aren't
# published for Ubuntu 24 (Noble) yet — same situation as Timescale —
# so we fall back to the Ubuntu-22 repo, which works because the
# kumomta binary links against system OpenSSL/libc and Noble's are
# forward-compatible with Jammy's.
#
# Default install ships /opt/kumomta/etc/policy/init.lua + a systemd
# unit; the daemon comes up bound to :25/:587/:465 with a minimal
# in-process config. The operator uses iris's "Apply policy" UI to
# overwrite init.lua with a real configuration.
# ──────────────────────────────────────────────────────────────────────
install_kumomta() {
    if systemctl list-unit-files kumomta.service >/dev/null 2>&1 \
        && command -v kumod >/dev/null 2>&1; then
        log "[4/6] KumoMTA already installed — skipping package install."
    else
        log "[4/6] Installing KumoMTA…"
        if [[ "$OS_FAMILY" == "apt" ]]; then
            apt-get install -y curl gnupg ca-certificates

            # Map the host codename to the repo openrepo publishes.
            # Hardcoded: kumomta only publishes for jammy + focal.
            local current_codename km_repo
            current_codename=$(lsb_release -c -s)
            case "$current_codename" in
                jammy)         km_repo=ubuntu22 ;;
                focal)         km_repo=ubuntu20 ;;
                noble|trixie|*) km_repo=ubuntu22
                    warn "kumomta doesn't publish for ${current_codename}; using ubuntu22 packages (forward-compatible)."
                    ;;
            esac

            # Keyring + sources file. Idempotent: --yes makes
            # gpg --dearmor overwrite without prompting; the .list is
            # rewritten unconditionally so the codename can change
            # between runs.
            curl -fsSL "https://openrepo.kumomta.com/kumomta-${km_repo/ubuntu/ubuntu-}/public.gpg" \
                | gpg --yes --dearmor -o /usr/share/keyrings/kumomta.gpg
            chmod 644 /usr/share/keyrings/kumomta.gpg
            curl -fsSL "https://openrepo.kumomta.com/files/kumomta-${km_repo}.list" \
                > /etc/apt/sources.list.d/kumomta.list

            apt-get update -y
            # The `kumomta` package is the stable channel; `kumomta-dev`
            # is the daily build. Stand-alone installs always want
            # stable.
            apt-get install -y kumomta
        else
            # Rocky 8/9 / RHEL 9 / Amazon Linux 2023 share one repo.
            $PKG install -y dnf-plugins-core || $PKG install -y yum-utils
            local km_repo_url
            if grep -qi 'amazon' /etc/os-release; then
                km_repo_url="https://openrepo.kumomta.com/files/kumomta-amazon2023.repo"
            else
                km_repo_url="https://openrepo.kumomta.com/files/kumomta-rocky.repo"
            fi
            if command -v dnf >/dev/null; then
                dnf config-manager --add-repo "$km_repo_url"
            else
                yum-config-manager --add-repo "$km_repo_url"
            fi
            $PKG install -y kumomta
        fi
    fi

    # Make sure the directories iris expects to write into exist
    # (the kumomta package creates them, but on a re-run with a
    # clobbered /opt/kumomta this is a safety net).
    mkdir -p /opt/kumomta/etc/policy /opt/kumomta/etc/dkim
    if getent group kumomta >/dev/null; then
        chgrp kumomta /opt/kumomta/etc/policy /opt/kumomta/etc/dkim 2>/dev/null || true
        chmod 0775   /opt/kumomta/etc/policy /opt/kumomta/etc/dkim 2>/dev/null || true
    fi

    # Capture kumod's diagnostic output (the ERROR/WARN/INFO lines — SMTP
    # rejections, log-hook failures, etc.) to a rotated file in addition to
    # journald. KumoMTA writes diagnostics to stderr; a systemd drop-in is
    # the cleanest way to tee that to disk without touching the upstream unit.
    # Verbosity itself is set in init.lua via kumo.set_diagnostic_log_filter
    # (rendered by iris), so this only controls the *destination*.
    mkdir -p /var/log/kumomta /etc/systemd/system/kumomta.service.d
    cat > /etc/systemd/system/kumomta.service.d/10-iris-diag-log.conf <<'EOF'
# Managed by iris install.sh — diagnostic log to file.
[Service]
StandardOutput=append:/var/log/kumomta/diagnostic.log
StandardError=append:/var/log/kumomta/diagnostic.log
EOF
    cat > /etc/logrotate.d/kumomta-diagnostic <<'EOF'
/var/log/kumomta/diagnostic.log {
    daily
    rotate 14
    missingok
    notifempty
    compress
    delaycompress
    copytruncate
}
EOF
    systemctl daemon-reload

    systemctl enable --now kumomta.service
    # daemon-reload above only rewrites the unit graph; a running kumod keeps
    # its old stderr fd until restarted, so bounce it to pick up the new
    # logging destination (no-op cost on first install where it just started).
    systemctl try-restart kumomta.service || true
    # The package's default init.lua may not configure the HTTP listener
    # iris talks to (127.0.0.1:8025). Without it iris's queue-management
    # calls will fail until the operator hits "Apply policy". That's a
    # documented next step, not a script bug.
    log "[4/6] KumoMTA running, diagnostics → /var/log/kumomta/diagnostic.log. (Use iris's UI 'Apply policy' to push a real init.lua.)"
}

# ──────────────────────────────────────────────────────────────────────
# Step 5 — Iris (deb or rpm from the GitHub release).
#
# Package layout (from deploy/nfpm/nfpm.yaml):
#   /usr/local/bin/iris
#   /etc/iris/iris.env (seeded from .example by postinstall)
#   /etc/iris/configs/{server,data,auth,kumo,logger}.yaml
#   /etc/iris/sql/*.sql
#   /usr/lib/systemd/system/iris.service
#   /var/lib/iris (state dir, currently unused)
# ──────────────────────────────────────────────────────────────────────
install_iris() {
    if command -v iris >/dev/null 2>&1 || [[ -x /usr/local/bin/iris ]]; then
        local current
        current=$(/usr/local/bin/iris --version 2>/dev/null || echo "unknown")
        log "[5/6] Iris already installed ($current). Re-installing $IRIS_VERSION to ensure pinned version."
    else
        log "[5/6] Installing Iris $IRIS_VERSION…"
    fi

    local ver="${IRIS_VERSION#v}"   # v0.1.0 → 0.1.0
    local tmp
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' RETURN

    if [[ "$OS_FAMILY" == "apt" ]]; then
        local pkg="iris_${ver}_${DEB_ARCH}.deb"
        local url="https://github.com/${IRIS_REPO}/releases/download/${IRIS_VERSION}/${pkg}"
        curl -fsSL -o "$tmp/$pkg" "$url"
        # `apt-get install ./file.deb` resolves dependencies; `dpkg -i`
        # alone leaves them broken. The :=$ver pin keeps a re-run from
        # silently downgrading.
        apt-get install -y "$tmp/$pkg"
    else
        local pkg="iris-${ver}-1.${RPM_ARCH}.rpm"
        local url="https://github.com/${IRIS_REPO}/releases/download/${IRIS_VERSION}/${pkg}"
        curl -fsSL -o "$tmp/$pkg" "$url"
        $PKG install -y "$tmp/$pkg"
    fi
}

# ──────────────────────────────────────────────────────────────────────
# Step 5 — wire iris.env + data.yaml to the local services.
#
# What gets touched:
#   /etc/iris/iris.env     JWT secrets (only if empty), metrics port
#   /etc/iris/configs/data.yaml   db source + redis source → 127.0.0.1
#
# JWT secrets are NEVER overwritten if already set — protects operator
# rotation. data.yaml is rewritten on every run (it's deterministic,
# nfpm flagged as config|noreplace so package upgrades won't touch it).
# ──────────────────────────────────────────────────────────────────────
configure_iris_env() {
    log "[6/6] Wiring iris.env + data.yaml…"

    local env=/etc/iris/iris.env
    [[ -f "$env" ]] || fail "$env missing — Iris install did not create it (postinstall failed?)."

    # Helper: set KEY=VALUE in $env. If KEY exists with empty value,
    # fill it. If KEY exists with a value, leave alone (preserves
    # operator edits). If KEY is missing, append.
    set_env_if_empty() {
        local key="$1" value="$2"
        if grep -qE "^${key}=$" "$env"; then
            # empty existing line — safe to fill
            sed -i "s|^${key}=$|${key}=${value//|/\\|}|" "$env"
        elif ! grep -qE "^${key}=" "$env"; then
            printf '%s=%s\n' "$key" "$value" >> "$env"
        fi
        # else: already populated — preserve.
    }

    # JWT secrets — generated once, never rotated by re-runs.
    set_env_if_empty IRIS_AUTH_ACCESS_SECRET  "$(openssl rand -base64 48 | tr -d '\n')"
    set_env_if_empty IRIS_AUTH_REFRESH_SECRET "$(openssl rand -base64 48 | tr -d '\n')"

    # Move the metrics listener off :9090 so prometheus-server can have
    # that port. 9091 is unprivileged and conventional for "secondary
    # exporter".
    if ! grep -qE '^IRIS_METRICS_LISTEN=' "$env"; then
        printf '\n# Override default metrics port to avoid colliding with prometheus-server on :9090.\nIRIS_METRICS_LISTEN=127.0.0.1:9091\n' >> "$env"
    fi

    # SQL migrations directory. The default look-up path inside the
    # binary (/app/sql) reflects the Docker image layout. On host
    # installs the package puts migrations under /etc/iris/sql.
    if ! grep -qE '^IRIS_SQL_MIGRATIONS_DIR=' "$env"; then
        printf '\n# Migrations dir — packaged at /etc/iris/sql; default path is the docker layout.\nIRIS_SQL_MIGRATIONS_DIR=/etc/iris/sql\n' >> "$env"
    fi

    # Local Prometheus URL — without this the dashboard endpoints
    # return 503 (handled but unhelpful for a stand-alone install
    # where the same script just installed Prometheus).
    if ! grep -qE '^IRIS_PROMETHEUS_URL=' "$env"; then
        printf '\n# Local Prometheus, scraped by Iris dashboard endpoints.\nIRIS_PROMETHEUS_URL=http://127.0.0.1:9090\n' >> "$env"
    fi

    # Restart-on-apply. kumomta loads listeners/relay_hosts and the log hook
    # only at init, so "Apply policy" must restart the daemon for those to
    # take effect. iris runs as a non-root, NoNewPrivileges=true unit, so sudo
    # (setuid) cannot escalate — we use plain `systemctl`, which only sends a
    # D-Bus request to PID 1; a polkit rule authorizes it by uid. No privilege
    # is gained inside the iris process, so this works under the hardened unit.
    if ! grep -qE '^IRIS_KUMO_RESTART_CMD=' "$env"; then
        printf '\n# Restart kumomta on policy apply so init-level changes (listeners,\n# relay_hosts, log hook) load. Authorized by the polkit rule below.\nIRIS_KUMO_RESTART_CMD=systemctl try-restart kumomta.service\n' >> "$env"
    fi
    # polkit rule: let the iris user manage ONLY kumomta.service, non-interactively.
    local polkit_rule=/etc/polkit-1/rules.d/49-iris-kumomta.rules
    mkdir -p /etc/polkit-1/rules.d
    cat > "$polkit_rule" <<'EOF'
// Managed by iris install.sh. Lets the iris service restart kumomta when a
// policy is applied (init-level changes only load at kumod start). Scoped to
// exactly kumomta.service and the iris user.
polkit.addRule(function(action, subject) {
    if (action.id == "org.freedesktop.systemd1.manage-units" &&
        action.lookup("unit") == "kumomta.service" &&
        subject.user == "iris") {
        return polkit.Result.YES;
    }
});
EOF
    chmod 0644 "$polkit_rule"
    # Pick up the new rule (polkit reloads rules.d on restart; reload is a no-op
    # if the daemon isn't running yet — it reads the dir on first start).
    systemctl restart polkit 2>/dev/null || systemctl restart polkitd 2>/dev/null || true

    # Lock down — env file holds JWT secrets.
    chown root:iris "$env" 2>/dev/null || true
    chmod 0640 "$env"

    # data.yaml swap: docker hostnames → loopback, with the generated
    # password baked into the connection string. The packaged file is
    # nfpm config|noreplace so we won't fight a package upgrade.
    local data=/etc/iris/configs/data.yaml
    [[ -f "$data" ]] || fail "$data missing — Iris install did not lay down configs/."

    local pw
    pw=$(< /etc/iris/.pgpassword)
    # url-encode the password so symbols don't break the connection
    # string. base64 minus / + = is already URL-safe so this is a
    # no-op for the default generator, but defensive against operators
    # who set IRIS_PG_PASSWORD with shell-special characters.
    local pw_enc
    pw_enc=$(python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$pw" 2>/dev/null) || pw_enc="$pw"

    # Replace just the source: lines — preserves the rest of the file.
    sed -i \
        -e "s|source: \"postgres://[^\"]*\"|source: \"postgres://${IRIS_PG_USER}:${pw_enc}@127.0.0.1:5432/${IRIS_PG_DB}?sslmode=disable\"|" \
        -e "s|source: \"redis://[^\"]*\"|source: \"${IRIS_REDIS_URL}\"|" \
        "$data"

    # Pre-create kumomta dirs so iris's ProtectSystem=strict +
    # ReadWritePaths doesn't stop the unit from starting before the
    # operator installs kumomta. They're harmless empty dirs.
    mkdir -p /opt/kumomta/etc/policy /opt/kumomta/etc/dkim /var/lib/iris
    # The unit runs as User=iris Group=kumomta; the kumomta group must
    # exist before the unit starts.
    if ! getent group kumomta >/dev/null; then
        groupadd --system kumomta
    fi
    chgrp kumomta /opt/kumomta/etc/policy /opt/kumomta/etc/dkim 2>/dev/null || true
    chmod 0775   /opt/kumomta/etc/policy /opt/kumomta/etc/dkim 2>/dev/null || true

    # Drop-in over the packaged unit. Two corrections:
    #
    # 1. After= — the packaged unit lists redis.service. Stand-alone
    #    installs use valkey-server.service instead. The packaged unit
    #    already lists kumomta.service, but we re-state it for clarity
    #    and to add postgresql.service which the package skips.
    #
    # 2. RestrictAddressFamilies — Kratos (and Go's net package
    #    generally) calls into netlink to enumerate interfaces at
    #    startup. The packaged unit's allowlist
    #    (AF_UNIX AF_INET AF_INET6) blocks AF_NETLINK, which makes
    #    the binary panic on first boot with
    #    `route ip+net: netlinkrib: address family not supported`.
    #    systemd merges drop-in RestrictAddressFamilies values with
    #    the unit's, so listing AF_NETLINK alone is additive.
    mkdir -p /etc/systemd/system/iris.service.d
    # Wants= kumomta only if the unit exists. Keeps the drop-in
    # valid on hosts where kumomta install was skipped (e.g.
    # KUMOMTA_SKIP=1 in a future revision).
    local kumo_after="" kumo_wants=""
    if systemctl list-unit-files kumomta.service >/dev/null 2>&1; then
        kumo_after=" kumomta.service"
        kumo_wants=$'\nWants=kumomta.service'
    fi

    cat > /etc/systemd/system/iris.service.d/standalone.conf <<EOF
# Drop-in installed by deploy/install.sh.

[Unit]
After=network-online.target $VALKEY_SERVICE postgresql.service${kumo_after}
Wants=$VALKEY_SERVICE${kumo_wants}

[Service]
RestrictAddressFamilies=AF_NETLINK
EOF

    systemctl daemon-reload
    log "[6/6] iris.env + data.yaml wired to local services."
}

# ──────────────────────────────────────────────────────────────────────
# Final boot — enable iris, give it a beat to come up, then summarise.
# ──────────────────────────────────────────────────────────────────────
start_iris() {
    log "Starting iris…"
    systemctl enable --now iris.service
    # Give the binary 5s to bind ports. If it crashes immediately the
    # next status check surfaces the failure.
    sleep 5
    if ! systemctl is-active --quiet iris.service; then
        warn "iris failed to start. Logs:"
        journalctl -u iris.service -n 50 --no-pager || true
        fail "iris did not come up cleanly — investigate the journal output above."
    fi
}

# ──────────────────────────────────────────────────────────────────────
# Verify the admin account got seeded by iris's auto-migrate, and
# optionally override the password from IRIS_ADMIN_PASSWORD.
#
# Iris's auto-migrate runs 0001_init.sql → 0004_*.sql in lexical order
# AFTER ent has created the schema. 0003_seed_admin.sql inserts
# username=admin / password=admin (bcrypt cost 12). Both 0002 + 0003
# are guarded with ON CONFLICT DO NOTHING, so re-runs are no-ops.
#
# We poll the users table for up to 30s (gives iris room to finish
# applying ent + sql migrations) before declaring failure. If
# IRIS_ADMIN_PASSWORD is set, we then UPDATE the password_hash with a
# bcrypt'd value via htpasswd.
# ──────────────────────────────────────────────────────────────────────
seed_admin_account() {
    log "Seeding admin account…"

    local pgpw deadline found
    pgpw=$(< /etc/iris/.pgpassword)
    deadline=$(($(date +%s) + 30))
    found=""

    # Poll for the admin row. iris's first-boot migration usually
    # completes in 1-3 seconds; the 30-second cap is generous.
    while [[ $(date +%s) -lt $deadline ]]; do
        # PGPASSWORD env passes the password without needing a
        # ~/.pgpass file. -tA strips column header + alignment so
        # the result is just "1" or empty.
        if [[ "$(PGPASSWORD="$pgpw" psql -h 127.0.0.1 -U "${IRIS_PG_USER}" -d "${IRIS_PG_DB}" -tA \
                    -c "SELECT 1 FROM users WHERE username='${IRIS_ADMIN_USERNAME}'" 2>/dev/null)" == "1" ]]; then
            found=1
            break
        fi
        sleep 2
    done

    if [[ -z "$found" ]]; then
        # iris's migrations didn't seed admin (likely 0003_seed_admin.sql
        # never ran, or the users table doesn't exist yet). Run the seed
        # SQL files explicitly. ent migration created the tables on iris
        # boot, so the schema is in place by now.
        warn "Admin user not found after 30s; running seed SQL files explicitly…"
        local seed_dir=/etc/iris/sql
        for f in "$seed_dir"/0002_seed_roles.sql "$seed_dir"/0003_seed_admin.sql; do
            [[ -f "$f" ]] || continue
            PGPASSWORD="$pgpw" psql -h 127.0.0.1 -U "${IRIS_PG_USER}" -d "${IRIS_PG_DB}" -v ON_ERROR_STOP=1 -f "$f" \
                || fail "Failed to apply $f — investigate before proceeding."
        done
        # Re-check.
        if [[ "$(PGPASSWORD="$pgpw" psql -h 127.0.0.1 -U "${IRIS_PG_USER}" -d "${IRIS_PG_DB}" -tA \
                    -c "SELECT 1 FROM users WHERE username='${IRIS_ADMIN_USERNAME}'" 2>/dev/null)" != "1" ]]; then
            fail "Admin account still missing after explicit seed — check iris logs (journalctl -u iris)."
        fi
    fi

    # Optional password override. The default 0003_seed_admin.sql sets
    # password=admin (bcrypt cost 12). If the operator provided a
    # different one via env, hash it and UPDATE.
    if [[ -n "$IRIS_ADMIN_PASSWORD" ]]; then
        # htpasswd ships in apache2-utils (apt) / httpd-tools (rpm).
        # Lazily install only when we actually need it.
        if ! command -v htpasswd >/dev/null 2>&1; then
            log "Installing htpasswd for password hashing…"
            if [[ "$OS_FAMILY" == "apt" ]]; then
                apt-get install -y apache2-utils
            else
                $PKG install -y httpd-tools
            fi
        fi
        # -B = bcrypt, -C 12 = cost 12 (matches auth.yaml's bcrypt_cost),
        # -n = print to stdout, -b = take password from CLI. Output
        # format is "user:hash"; strip the username prefix.
        local hash
        hash=$(htpasswd -B -C 12 -n -b "${IRIS_ADMIN_USERNAME}" "$IRIS_ADMIN_PASSWORD" 2>/dev/null \
            | sed 's/^[^:]*://') \
            || fail "htpasswd failed to hash the admin password."
        # Single-quote escape for the SQL literal.
        local hash_sql=${hash//\'/\'\'}
        PGPASSWORD="$pgpw" psql -h 127.0.0.1 -U "${IRIS_PG_USER}" -d "${IRIS_PG_DB}" -v ON_ERROR_STOP=1 \
            -c "UPDATE users SET password_hash='${hash_sql}', updated_at=NOW() WHERE username='${IRIS_ADMIN_USERNAME}'" \
            >/dev/null \
            || fail "Failed to update admin password."
        log "Admin password set from IRIS_ADMIN_PASSWORD."
        IRIS_ADMIN_DISPLAY_PASSWORD="(set via IRIS_ADMIN_PASSWORD)"
    else
        IRIS_ADMIN_DISPLAY_PASSWORD="admin  ⚠  ROTATE IMMEDIATELY"
    fi
}

print_summary() {
    cat <<EOF

──────────────────────────────────────────────────────────────────────
Iris stand-alone install complete.

Services:
  - Valkey       systemctl status $VALKEY_SERVICE
  - PostgreSQL   systemctl status postgresql*
  - Prometheus   systemctl status prometheus      ➜ http://127.0.0.1:9090
  - KumoMTA      systemctl status kumomta         ➜ smtp on :25, :587, :465
  - Iris         systemctl status iris            ➜ http://127.0.0.1:8000

Database:
  ${IRIS_PG_DB} owned by ${IRIS_PG_USER}; password stored at /etc/iris/.pgpassword (root-only).

Login:
  username: ${IRIS_ADMIN_USERNAME}
  password: ${IRIS_ADMIN_DISPLAY_PASSWORD}

Next steps:
  1. Browse to http://<this-host>:8000 — login with the credentials above.
  2. Rotate the admin password from Profile → Change Password.
  3. In the iris UI, open Policy → Apply to push a real KumoMTA init.lua
     (the default kumomta init.lua has no HTTP listener, so iris's queue
     management calls won't work until you apply a policy).

Logs:  journalctl -u iris -f
Edit:  sudo \$EDITOR /etc/iris/iris.env  &&  sudo systemctl restart iris
──────────────────────────────────────────────────────────────────────
EOF
}

# ──────────────────────────────────────────────────────────────────────
# main
# ──────────────────────────────────────────────────────────────────────
main() {
    detect_os
    install_valkey
    install_timescaledb
    init_iris_database
    install_prometheus
    install_kumomta
    install_iris
    configure_iris_env
    start_iris
    seed_admin_account
    print_summary
}

main "$@"
