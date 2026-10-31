#!/usr/bin/env bash
set -euo pipefail

REPO="ronel99/kubectl-argo"
BINARY_NAME="kubectl-argo"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${KUBECTL_ARGO_VERSION:-}"

info() {
  printf "\033[1;34m==>\033[0m %s\n" "$*"
}

success() {
  printf "\033[1;32m✓\033[0m %s\n" "$*"
}

error() {
  printf "\033[1;31mError:\033[0m %s\n" "$*" >&2
  exit 1
}

detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "${os}" in
    linux) echo "linux" ;;
    darwin) echo "darwin" ;;
    *) error "Unsupported operating system '${os}'. Use install.ps1 on Windows." ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  case "${arch}" in
    x86_64|amd64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) error "Unsupported architecture '${arch}'. Supported architectures: amd64, arm64." ;;
  esac
}

resolve_version() {
  if [ -n "${VERSION}" ]; then
    echo "${VERSION}"
    return
  fi

  local latest
  latest="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"
  if [ -z "${latest}" ]; then
    error "Failed to determine latest release version from GitHub API. Set KUBECTL_ARGO_VERSION explicitly."
  fi
  echo "${latest}"
}

verify_checksum() {
  local dir="$1"
  local archive="$2"

  if [ ! -f "${dir}/checksums.txt" ]; then
    return 0
  fi

  info "Verifying SHA-256 checksum..."
  local expected
  expected="$(grep "  ${archive}\$" "${dir}/checksums.txt" | awk '{print $1}' || true)"
  if [ -z "${expected}" ]; then
    return 0
  fi

  local actual=""
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${dir}/${archive}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${dir}/${archive}" | awk '{print $1}')"
  fi

  if [ -n "${actual}" ] && [ "${expected}" != "${actual}" ]; then
    error "Checksum mismatch for ${archive}! Expected ${expected}, got ${actual}."
  fi
}

main() {
  command -v curl >/dev/null 2>&1 || error "'curl' is required to install ${BINARY_NAME}."
  command -v tar >/dev/null 2>&1 || error "'tar' is required to extract ${BINARY_NAME}."

  local os arch version archive base_url tmp_dir
  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(resolve_version)"
  archive="${BINARY_NAME}_${version}_${os}_${arch}.tar.gz"
  base_url="https://github.com/${REPO}/releases/download/${version}"

  tmp_dir="$(mktemp -d)"
  trap "rm -rf '${tmp_dir}'" EXIT

  info "Downloading ${BINARY_NAME} ${version} for ${os}/${arch}..."
  curl -fsSL "${base_url}/${archive}" -o "${tmp_dir}/${archive}"
  curl -fsSL "${base_url}/checksums.txt" -o "${tmp_dir}/checksums.txt" 2>/dev/null || true

  verify_checksum "${tmp_dir}" "${archive}"

  tar -xzf "${tmp_dir}/${archive}" -C "${tmp_dir}" "${BINARY_NAME}"
  chmod +x "${tmp_dir}/${BINARY_NAME}"

  if [ -w "${INSTALL_DIR}" ]; then
    mv "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  elif command -v sudo >/dev/null 2>&1; then
    info "Installing ${BINARY_NAME} to ${INSTALL_DIR} (requires sudo)..."
    sudo mkdir -p "${INSTALL_DIR}"
    sudo mv "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "${INSTALL_DIR}"
    mv "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    info "Installed to ${INSTALL_DIR}/${BINARY_NAME} (ensure ${INSTALL_DIR} is in your PATH)"
  fi

  success "Successfully installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
  "${INSTALL_DIR}/${BINARY_NAME}" version || true
}

main "$@"
