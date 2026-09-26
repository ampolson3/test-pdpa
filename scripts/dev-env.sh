#!/bin/sh
# Creates .env from .env.example (if missing) and fills the two values that must be random even in development:
# AUTH_SECRET (Auth.js session encryption) and LOCAL_KEK_BASE64 (PLT-13 dev master key). Safe to re-run.
set -eu
cd "$(dirname "$0")/.."
[ -f .env ] || { cp .env.example .env; echo "created .env from .env.example"; }

fill() { # fill KEY with a fresh random value when it is empty or still the example placeholder
  key=$1
  current=$(grep -E "^$key=" .env | head -n 1 | cut -d= -f2- || true)
  case "$current" in
    "" | change-me*)
      value=$(openssl rand -base64 32)
      if grep -qE "^#? ?$key=" .env; then
        awk -v k="$key" -v v="$value" 'BEGIN{done=0} { if (!done && ($0 ~ "^" k "=" || $0 ~ "^# ?" k "=")) { print k "=" v; done=1 } else print }' .env > .env.tmp && mv .env.tmp .env
      else
        printf '%s=%s\n' "$key" "$value" >> .env
      fi
      echo "set $key" ;;
  esac
}
fill AUTH_SECRET
fill LOCAL_KEK_BASE64
