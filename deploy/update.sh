#!/bin/sh

set -eu

REPOSITORY="CoLinkDev/colink-server"
MODE="update"
VERSION=""
MANAGED_FILES="
docker-compose.yml
.env.example
deploy/nginx/default.conf
"

log() {
    printf '%s\n' "$*"
}

fail() {
    printf 'Error: %s\n' "$*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: update.sh --version VERSION [--check]

Run this command from the directory used for the CoLink deployment. The
directory may be empty for a new deployment or contain an existing deployment.

Options:
  --version VERSION  Release tag to install, such as v1.0.0 (required).
  --check            Report outdated deployment files without changing them.
  -h, --help         Show this help text.
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version)
            [ "$#" -ge 2 ] || fail "--version requires a value"
            VERSION="$2"
            shift 2
            ;;
        --check)
            MODE="check"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            fail "unknown argument: $1"
            ;;
    esac
done

[ -n "$VERSION" ] || fail "--version is required (for example: --version v1.0.0)"
printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$' \
    || fail "invalid version: $VERSION (expected a release tag such as v1.0.0)"
version_without_prefix=${VERSION#v}
version_major=${version_without_prefix%%.*}
[ "$version_major" -ge 1 ] || fail "unsupported version: $VERSION (v1.0.0 or later is required)"

DEPLOY_DIR=$(pwd -P)
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v awk >/dev/null 2>&1 || fail "awk is required"
command -v docker >/dev/null 2>&1 || fail "docker is required"
docker compose version >/dev/null 2>&1 || fail "docker compose is required"

TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/colink-deploy-update.XXXXXX")
trap 'rm -rf "$TEMP_DIR"' EXIT HUP INT TERM

fetch_file() {
    fetch_relative_path="$1"
    fetch_destination="$TEMP_DIR/files/$fetch_relative_path"
    mkdir -p "$(dirname "$fetch_destination")"

    curl -fsSL --retry 3 \
        "https://raw.githubusercontent.com/$REPOSITORY/$VERSION/$fetch_relative_path" \
        -o "$fetch_destination" \
        || fail "could not download $fetch_relative_path for version $VERSION"

    [ -s "$fetch_destination" ] || fail "downloaded file is empty: $fetch_relative_path"
}

set_env_value() {
    env_file="$1"
    env_key="$2"
    env_value="$3"
    updated_env_file="$TEMP_DIR/env.updated"

    awk -v key="$env_key" -v value="$env_value" '
        index($0, key "=") == 1 {
            if (!found) {
                print key "=" value
                found = 1
            }
            next
        }
        { print }
        END {
            if (!found) {
                print ""
                print key "=" value
            }
        }
    ' "$env_file" > "$updated_env_file"

    mv "$updated_env_file" "$env_file"
}

migrate_env() {
    migration_env_file="$1"

    set_env_value "$migration_env_file" "COLINK_SERVER_IMAGE_TAG" "${VERSION#v}"

    # Add future migrations here as ordered, idempotent edits. A migration must
    # preserve existing values and secrets, and must be safe to run repeatedly.
    # This function receives a staged copy; the live .env is replaced only after
    # every downloaded file and Docker configuration has been validated.
    : "$migration_env_file"
}

create_initial_env() {
    initial_env_example="$1"
    initial_env_destination="$2"
    initial_env_secret_file="$TEMP_DIR/jwt-secret"

    [ -r /dev/urandom ] || fail "/dev/urandom is required to generate COLINK_JWT_SECRET"
    command -v od >/dev/null 2>&1 || fail "od is required to generate COLINK_JWT_SECRET"

    (od -An -N32 -tx1 /dev/urandom | tr -d ' \n'; printf '\n') > "$initial_env_secret_file"
    chmod 600 "$initial_env_secret_file"
    grep -Eq '^[0-9a-f]{64}$' "$initial_env_secret_file" \
        || fail "could not generate COLINK_JWT_SECRET"

    awk -v secret_file="$initial_env_secret_file" '
        BEGIN {
            if ((getline secret < secret_file) <= 0) {
                exit 2
            }
            close(secret_file)
            found = 0
        }
        /^COLINK_JWT_SECRET=/ {
            print "COLINK_JWT_SECRET=" secret
            found = 1
            next
        }
        { print }
        END {
            if (!found) {
                exit 3
            }
        }
    ' "$initial_env_example" > "$initial_env_destination" \
        || fail "COLINK_JWT_SECRET is missing from .env.example"
}

for relative_path in $MANAGED_FILES; do
    fetch_file "$relative_path"
done

if [ -f "$DEPLOY_DIR/.env" ]; then
    cp "$DEPLOY_DIR/.env" "$TEMP_DIR/migrated.env"
    env_is_new=0
else
    create_initial_env "$TEMP_DIR/files/.env.example" "$TEMP_DIR/migrated.env"
    env_is_new=1
fi
migrate_env "$TEMP_DIR/migrated.env"

docker compose \
    --env-file "$TEMP_DIR/migrated.env" \
    -f "$TEMP_DIR/files/docker-compose.yml" \
    config --quiet

docker run --rm \
    -v "$TEMP_DIR/files/deploy/nginx/default.conf:/etc/nginx/conf.d/default.conf:ro" \
    nginx:1.27-alpine nginx -t >/dev/null

updates_needed=0
for relative_path in $MANAGED_FILES; do
    if [ ! -f "$DEPLOY_DIR/$relative_path" ] || ! cmp -s "$TEMP_DIR/files/$relative_path" "$DEPLOY_DIR/$relative_path"; then
        log "outdated: $relative_path"
        updates_needed=1
    else
        log "current:  $relative_path"
    fi
done

if [ "$env_is_new" -eq 1 ]; then
    log "missing:  .env"
    updates_needed=1
elif ! cmp -s "$TEMP_DIR/migrated.env" "$DEPLOY_DIR/.env"; then
    log "migration required: .env"
    updates_needed=1
else
    log "current:  .env"
fi

if [ "$MODE" = "check" ]; then
    if [ "$updates_needed" -eq 0 ]; then
        log "Deployment files are current for version $VERSION."
        exit 0
    fi
    log "Deployment file updates are available for version $VERSION."
    exit 1
fi

if [ "$updates_needed" -eq 0 ]; then
    log "Nothing to update."
    exit 0
fi

backup_dir=""

ensure_backup_dir() {
    [ -z "$backup_dir" ] || return 0
    timestamp=$(date -u '+%Y%m%dT%H%M%SZ')
    backup_dir="$DEPLOY_DIR/.colink-deploy-backups/$timestamp-$$"
    mkdir -p "$backup_dir"
    chmod 700 "$DEPLOY_DIR/.colink-deploy-backups" "$backup_dir"
}

backup_file() {
    backup_relative_path="$1"
    backup_source="$DEPLOY_DIR/$backup_relative_path"
    [ -f "$backup_source" ] || return 0
    ensure_backup_dir
    backup_destination="$backup_dir/$backup_relative_path"
    mkdir -p "$(dirname "$backup_destination")"
    cp -p "$backup_source" "$backup_destination"
}

replace_file() {
    replace_relative_path="$1"
    replace_source="$2"
    replace_mode="$3"
    replace_destination="$DEPLOY_DIR/$replace_relative_path"
    mkdir -p "$(dirname "$replace_destination")"
    backup_file "$replace_relative_path"

    if [ -f "$replace_destination" ]; then
        # Preserve the inode so running Docker bind mounts observe the update.
        cp "$replace_source" "$replace_destination"
    else
        cp "$replace_source" "$replace_destination"
        chmod "$replace_mode" "$replace_destination"
    fi
    log "updated:  $replace_relative_path"
}

for relative_path in $MANAGED_FILES; do
    if [ ! -f "$DEPLOY_DIR/$relative_path" ] || ! cmp -s "$TEMP_DIR/files/$relative_path" "$DEPLOY_DIR/$relative_path"; then
        replace_file "$relative_path" "$TEMP_DIR/files/$relative_path" 644
    fi
done

if [ "$env_is_new" -eq 1 ] || ! cmp -s "$TEMP_DIR/migrated.env" "$DEPLOY_DIR/.env"; then
    replace_file ".env" "$TEMP_DIR/migrated.env" 600
fi

if [ -n "$backup_dir" ]; then
    log "Backup: $backup_dir"
fi

if [ "$env_is_new" -eq 1 ]; then
    log "Created .env with a generated COLINK_JWT_SECRET. Review optional settings as needed."
else
    log "Deployment files were updated to version $VERSION."
fi
log "Services were not restarted. Apply the deployment with docker compose pull and docker compose up -d."
