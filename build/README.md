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
`appicon.png`, `darwin/icons.icns` (the macOS bundle's icon, made from the png
by `wails3 generate icons`), `windows/icon.ico` and the Linux packages'
`nfpm/icons/`. The outputs are committed; re-run it after changing either SVG.

## Mac

The `darwin` directory holds the files of the macOS bundle.

- `Info.plist` - the bundle's property list, a template: `scripts/build/stamp.mjs`
  fills the product facts in from `wails.json` when `make build-darwin` makes
  the bundle.
- `Info.dev.plist` - the same for the development bundle `make dev` runs from
  (it lets the app reach the Vite dev server over the local network).
- `icons.icns` - the bundle's icon (see above).

## Windows

The `windows` directory holds the files of the Windows build.

- `icon.ico` - The icon used for the application and its installer, rendered by `make app-icons` (see above).
- `info.json` - Application details, a template stamped from `wails.json`. The data here is the exe's
  properties (right click the exe -> properties -> details) and is compiled into its resource file by
  `wails3 generate syso`.
- `wails.exe.manifest` - The application manifest, also a template, compiled into the same resource file.
- `installer/*` - The files used to create the Windows installer: `project.nsi`, the script `makensis`
  compiles, and `wails_tools.nsh`, the include file it uses, whose product facts
  `scripts/build/installer-tools.mjs` writes from `wails.json`.

## Development

- `config.yml` - what `wails3 dev` (`make dev`) runs: the build, the Vite dev server and the app, which
  are the tasks in the `Taskfile.yml` at the root of the repository.
