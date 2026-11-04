# Homebrew

Install the macOS Apple Silicon release:

```sh
brew install --cask mreasonyang/taps/ytmgo
```

This uses [mreasonyang/homebrew-taps](https://github.com/mreasonyang/homebrew-taps/blob/main/Casks/ytmgo.rb), requires macOS 27 or newer and Apple Silicon, and installs `ytmGO.app`. The cask downloads the notarized release ZIP and verifies its SHA-256. No source build or additional runtime installation is required.

```sh
brew update
brew upgrade --cask --greedy mreasonyang/taps/ytmgo
```

`--greedy` includes this cask even though it also provides in-app automatic updates. Quit ytmGO before changing installation methods. If a manual installation already exists, back it up and move it out of `/Applications` before the first Homebrew install; do not use `--force` to overwrite it. Regular `brew uninstall --cask ytmgo` retains account data and preferences.

## Maintaining the tap

The tap's **Update ytmGO cask** workflow checks the latest public stable release every six hours and can also be dispatched after a release. It needs only its own repository's built-in `GITHUB_TOKEN` to commit the cask; release downloads are anonymous. No extra cross-repository token is required.

The generator accepts stable `0.6.X` releases starting at `0.6.3`, refuses downgrades, checks the source revision, signing/notarization/update flags and platform in `build.json`, and requires the ZIP digest to match both GitHub's asset digest and the published `SHA256SUMS`. Metadata downloads are also checked against their asset digests. A change to the release version series, bundle name or minimum macOS requires reviewing the generator and cask together.

For immediate synchronization, run the tap workflow after the notarized release is public:

```sh
gh workflow run update-ytmgo.yml --repo mreasonyang/homebrew-taps
```

The [Homebrew Cask Cookbook](https://docs.brew.sh/Cask-Cookbook) describes the cask fields; [tap maintenance](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap) describes the fully qualified install command.
