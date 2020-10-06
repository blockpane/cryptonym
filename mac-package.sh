#!/bin/bash

# builds a macos package (.app) and places it inside a compressed .dmg

mkdir -p "package/Cryptonym"
mkdir -p package/old
mv package/*.dmg package/old/

go build -ldflags "-s -w" -o cmd/cryptonym-wallet/cryptonym-wallet cmd/cryptonym-wallet/main.go

upx -9 cmd/cryptonym-wallet/cryptonym-wallet
fyne package -sourceDir cmd/cryptonym-wallet -name "Cryptonym" -os darwin && mv "Cryptonym.app" "package/Cryptonym/"
sed -i'' -e 's/.string.1\.0.\/string./\<string>'$(git describe --tags --always --long)'\<\/string>/g' "package/Cryptonym/Cryptonym.app/Contents/Info.plist"
rm -f cmd/cryptonym-wallet/cryptonym-wallet
pushd package
hdiutil create -srcfolder "Cryptonym" "Cryptonym.dmg"
popd
rm -fr "package/Cryptonym"
open "package/Cryptonym.dmg"

