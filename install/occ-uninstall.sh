#!/bin/bash
# Based on Deno and nvm uninstaller: Copyright 2023 the Deno authors. All rights reserved. MIT license.
# Keep this script simple and easily auditable.
set -e

main() {
    local OCC_DIR=~/.occ
    local OCC_BIN_DIR=$OCC_DIR/bin
    local OCC_CLI_EXEC=$OCC_BIN_DIR/occ

    echo "Uninstalling occ..."

    # 1. Check if occ is installed
    if [ ! -d "$OCC_DIR" ]; then
        echo "occ does not appear to be installed (directory $OCC_DIR not found)"
        exit 1
    fi

    # 2. Remove occ binaries and directory
    echo "Removing occ binaries and directories..."
    rm -rf "$OCC_DIR"

    # 3. Clean up profile file
    local PROFILE=$(detect_profile)

    if [ -z "$PROFILE" ]; then
        echo "No profile detected"
        echo "If you manually added occ to your PATH, please remove these lines from your shell profile:"
        echo "export OCCTL_DIR=$OCC_DIR"
        echo "export PATH=$OCC_DIR/bin:\${PATH}"
        echo "[ -f \$OCCTL_DIR/bin/occ-completion ] && source \$OCCTL_DIR/bin/occ-completion"
    else
        echo "Detected profile: $PROFILE"
        echo "Cleaning up $PROFILE..."

        # Create backup of profile file
        cp "$PROFILE" "${PROFILE}.bak.$(date +%Y%m%d%H%M%S)"

        # Remove occ-related lines
        grep -v "OCCTL_DIR" "$PROFILE" | grep -v "occ-completion" > "${PROFILE}.tmp"
        mv "${PROFILE}.tmp" "$PROFILE"

        # Fix PATH to remove occ path
        sed -i.bak "s|$OCC_BIN_DIR:||g" "$PROFILE"
        rm -f "${PROFILE}.bak"

        echo "Removed occ references from $PROFILE"
    fi

    # 4. Verify uninstallation
    if [ -d "$OCC_DIR" ]; then
        echo "Warning: Could not completely remove $OCC_DIR"
    else
        echo "occ was uninstalled successfully 🎉"
        echo "To complete the uninstallation, please restart your terminal or run: source $PROFILE"
    fi
}

detect_profile() {
    if [ "${PROFILE-}" = '/dev/null' ]; then
        # the user has specifically requested NOT to touch their profile
        return
    fi

    if [ -n "${PROFILE}" ] && [ -f "${PROFILE}" ]; then
        echo "${PROFILE}"
        return
    fi

    local DETECTED_PROFILE
    DETECTED_PROFILE=''

    if [ "${SHELL#*bash}" != "$SHELL" ]; then
        if [ -f "$HOME/.bashrc" ]; then
            DETECTED_PROFILE="$HOME/.bashrc"
        elif [ -f "$HOME/.bash_profile" ]; then
            DETECTED_PROFILE="$HOME/.bash_profile"
        fi
    elif [ "${SHELL#*zsh}" != "$SHELL" ]; then
        if [ -f "$HOME/.zshrc" ]; then
            DETECTED_PROFILE="$HOME/.zshrc"
        elif [ -f "$HOME/.zprofile" ]; then
            DETECTED_PROFILE="$HOME/.zprofile"
        fi
    fi

    if [ -z "$DETECTED_PROFILE" ]; then
        if [ -f "$HOME/.profile" ]; then
            DETECTED_PROFILE="$HOME/.profile"
        elif [ -f "$HOME/.bashrc" ]; then
            DETECTED_PROFILE="$HOME/.bashrc"
        elif [ -f "$HOME/.bash_profile" ]; then
            DETECTED_PROFILE="$HOME/.bash_profile"
        elif [ -f "$HOME/.zshrc" ]; then
            DETECTED_PROFILE="$HOME/.zshrc"
        elif [ -f "$HOME/.zprofile" ]; then
            DETECTED_PROFILE="$HOME/.zprofile"
        fi
    fi

    if [ ! -z "$DETECTED_PROFILE" ]; then
        echo "$DETECTED_PROFILE"
    fi
}

# Display a confirmation prompt
confirm_uninstall() {
    read -p "Are you sure you want to uninstall occ? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Uninstallation cancelled."
        exit 0
    fi
}

# Ask for confirmation before proceeding
confirm_uninstall

# Run the main function
main "$@"
unset -f main detect_profile confirm_uninstall
