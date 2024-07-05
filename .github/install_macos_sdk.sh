#!/usr/bin/env bash
# Install an older MacOS SDK
# This should guarantee OpenMM builds with extended compatibility across MacOS versions
# Taken from
# https://github.com/openmm/openmm/blob/master/devtools/ci/gh-actions/scripts/install_macos_sdk.sh

OSX_SDK_DIR="$(xcode-select -p)/Platforms/MacOSX.platform/Developer/SDKs"
export MACOSX_DEPLOYMENT_TARGET=10.15
export MACOSX_SDK_VERSION=10.15

export OSX_SYSROOT="${OSX_SDK_DIR}/MacOSX${MACOSX_SDK_VERSION}.sdk"

if [[ ! -d ${OSX_SYSROOT}} ]]; then
    echo "Downloading ${MACOSX_SDK_VERSION} sdk"
    curl -L -O --connect-timeout 5 --max-time 10 --retry 5  --retry-delay 0 --retry-max-time 40 --retry-connrefused --retry-all-errors \
        https://github.com/phracker/MacOSX-SDKs/releases/download/10.15/MacOSX${MACOSX_SDK_VERSION}.sdk.tar.xz
    tar -xf MacOSX${MACOSX_SDK_VERSION}.sdk.tar.xz -C "$(dirname ${OSX_SYSROOT})"
fi

if [[ "$MACOSX_DEPLOYMENT_TARGET" == 10.* ]]; then
# set minimum sdk version to our target
plutil -replace MinimumSDKVersion -string ${MACOSX_SDK_VERSION} $(xcode-select -p)/Platforms/MacOSX.platform/Info.plist
plutil -replace DTSDKName -string macosx${MACOSX_SDK_VERSION}internal $(xcode-select -p)/Platforms/MacOSX.platform/Info.plist
fi


echo "SDKROOT=${OSX_SYSROOT}" >> ${GITHUB_ENV}