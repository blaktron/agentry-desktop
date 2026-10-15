# Build Directory

The build directory holds the build files and assets of the application.

The structure is:

* bin - Output directory
* darwin - macOS specific files
* windows - Windows specific files
* nfpm - the Linux package definition (`nfpm.yaml`), the desktop entry and the
  rendered hicolor icons
* cli - where a release puts the signed `agentry` CLI that the packages carry
  (ignored by git)

## The app icon

`appicon.svg` (the Agentry mark on its tile) and `appicon-small.svg` (the bare
mark, for 16 to 32 pixels) are the sources. `make app-icons` renders
`appicon.png`, which Wails turns into the macOS bundle's `iconfile.icns`,
`windows/icon.ico` and the Linux packages' `nfpm/icons/`. The outputs are
committed; re-run it after changing either SVG.

## Mac

The `darwin` directory holds files specific to Mac builds.
These may be customised and used as part of the build. To return these files to the default state, simply delete them
and
build with `wails build`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds. It is used when building using `wails build`.
- `Info.dev.plist` - same as the main plist file but used when building using `wails dev`.

## Windows

The `windows` directory contains the manifest and rc files used when building with `wails build`.
These may be customised for your application. To return these files to the default state, simply delete them and
build with `wails build`.

- `icon.ico` - The icon used for the application and its installer, rendered by `make app-icons` (see above).
- `installer/*` - The files used to create the Windows installer. These are used when building using `wails build`.
- `info.json` - Application details used for Windows builds. The data here will be used by the Windows installer,
  as well as the application itself (right click the exe -> properties -> details)
- `wails.exe.manifest` - The main application manifest file.