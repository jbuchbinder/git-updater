# GIT-UPDATER

Updates large directory full of git repositories with status and error handling

## Build and install

    make install        # builds ./git-updater and installs to ~/.local/bin

`PREFIX` selects the install directory (`make install PREFIX=/usr/local/bin`).
Run `make` inside the directory that holds your repositories; every immediate
subdirectory containing a `.git` directory is pulled in parallel.

Install after every source change: several copies of the binary can exist
(`~/go/bin`, `~/go.old/bin`), and a stale one earlier in `PATH` will keep the
old behaviour while the tree looks fixed.

