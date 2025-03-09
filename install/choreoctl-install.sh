#!/bin/bash
# Based on Deno and nvm installer: Copyright 2023 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.
set -e

getArchitecture() {
    local ARCH=$(uname -m | tr '[:upper:]' '[:lower:]')
    if [[ "$ARCH" == "x86_64" ]]; then
        echo "amd64"
    elif [[ "$ARCH" == "i386" ]]; then
        echo "386"
    elif [[ "$ARCH" == "arm64" || "$ARCH" == "aarch64" ]]; then
        echo "arm64"
    elif [[ "$ARCH" == "arm" ]]; then
        echo "arm"
    else
        echo "Unsupported architecture: $ARCH"
        exit 1
    fi
}

main() {
    local OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    local ARCH=$(getArchitecture)
    local SHELL_TYPE=$(basename $SHELL)
    local CHOREO_DIR=~/.choreoctl
    local CHOREO_BIN_DIR=$CHOREO_DIR/bin
    local CHOREO_CLI_EXEC=$CHOREO_BIN_DIR/choreoctl
    local DIST_DIR="dist/choreoctl"

    mkdir -p $CHOREO_BIN_DIR

    # Check if dist directory exists
    if [ ! -d "$DIST_DIR" ]; then
        echo "Error: dist directory not found. Please run 'make dist-all' first."
        exit 1
    fi

    # Check if binary exists for current platform
    local PLATFORM_DIR="$DIST_DIR/$OS-$ARCH"
    if [ ! -d "$PLATFORM_DIR" ]; then
        echo "Error: No binary found for $OS-$ARCH platform"
        exit 1
    fi

    echo "Installing choreoctl..."
    echo "Copying executable from dist directory..."

    cp "$PLATFORM_DIR/choreoctl" "$CHOREO_CLI_EXEC"
    chmod +x "$CHOREO_CLI_EXEC"

    cd $CHOREO_BIN_DIR
    touch ./choreoctl-completion

    ./choreoctl completion $SHELL_TYPE > ./choreoctl-completion
    chmod +x ./choreoctl-completion

    local PROFILE=$(detect_profile)

    if [ -z $PROFILE ]; then
        echo "No profile detected"
        echo "Please add the following lines at the beginning of your shell profile:"
        echo "export CHOREOCTL_DIR=$CHOREO_DIR"
        echo "export PATH=$CHOREO_DIR/bin:\${PATH}"
        echo "[ -f \$CHOREOCTL_DIR/bin/choreoctl-completion ] && source \$CHOREOCTL_DIR/bin/choreoctl-completion"
    else
        echo "Detected profile: $PROFILE"
        if ! grep -qc "$CHOREO_DIR" "$PROFILE"; then
            echo "Adding choreoctl to PATH in $PROFILE"
            # Add to beginning of PATH to take precedence
            sed -i.bak "1i\\
export CHOREOCTL_DIR=$CHOREO_DIR\\
export PATH=$CHOREO_DIR/bin:\${PATH}\\
[ -f \$CHOREOCTL_DIR/bin/choreoctl-completion ] && source \$CHOREOCTL_DIR/bin/choreoctl-completion
" "$PROFILE"
            rm "${PROFILE}.bak"
        else
            echo "choreoctl is already in PATH"
        fi
    fi

    # Add verification step
    echo "Verifying installation..."
    if [ -x "$CHOREO_CLI_EXEC" ]; then
        echo "choreoctl was installed successfully 🎉"

        # Try to update PATH in current session
        export CHOREOCTL_DIR=$CHOREO_DIR
        export PATH=$CHOREO_DIR/bin:${PATH}

        # Check if it's accessible in current session
        if command -v choreoctl >/dev/null 2>&1; then
            echo "choreoctl is ready to use in your current terminal session"
        else
            echo "To use choreoctl in this terminal session, please run:"
            if [ -n "$PROFILE" ]; then
                echo "  source $PROFILE"
            else
                echo "  export PATH=$CHOREO_DIR/bin:\$PATH"
            fi
            echo "Or start a new terminal session"
        fi
    else
        echo "Warning: Installation may have failed. The executable at $CHOREO_CLI_EXEC is not found or not executable."
        echo "Please check the error messages above and try again."
    fi
}


detect_profile() {
    if [ "${PROFILE-}" = '/dev/null' ]; then
        # the user has specifically requested NOT to touch their profile
        return
    fi

    if [ -n "${PROFILE}" ] && [ -f "${PROFILE}" ]; then
        nvm_echo "${PROFILE}"
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


main "$@"
unset -f main detect_profile getArchitecture
