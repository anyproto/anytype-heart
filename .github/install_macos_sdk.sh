#!/usr/bin/env bash
# Install an older MacOS SDK

OSX_SDK_DIR="$(xcode-select -p)/Platforms/MacOSX.platform/Developer/SDKs"
export MACOSX_DEPLOYMENT_TARGET=$1
export MACOSX_SDK_VERSION=$MACOSX_DEPLOYMENT_TARGET

export OSX_SYSROOT="${OSX_SDK_DIR}/MacOSX${MACOSX_SDK_VERSION}.sdk"
FILENAME="MacOSX${MACOSX_SDK_VERSION}.sdk.tar.xz"
DOWNLOAD_URL="https://github.com/phracker/MacOSX-SDKs/releases/download/10.15/${FILENAME}"

if [[ ! -d ${OSX_SYSROOT}} ]]; then
    echo "MacOS SDK ${MACOSX_SDK_VERSION} is missing, downloading..."
    curl -L -O --connect-timeout 5 --max-time 10 --retry 10  --retry-delay 0 --retry-max-time 40 --retry-connrefused --retry-all-errors \
        ${DOWNLOAD_URL}
    tar -xf ${FILENAME} -C "$(dirname ${OSX_SYSROOT})"
fi

plutil -replace MinimumSDKVersion -string ${MACOSX_SDK_VERSION} $(xcode-select -p)/Platforms/MacOSX.platform/Info.plist
plutil -replace DTSDKName -string macosx${MACOSX_SDK_VERSION}internal $(xcode-select -p)/Platforms/MacOSX.platform/Info.plist

echo "SDKROOT=${OSX_SYSROOT}" >> ${GITHUB_ENV}