# ======================================================================
# nix-shell expression encapuslating all developing voyager under --pure
# for an interactive shell: nix-shell --pure --keep GOPATH
# to run a command: nix-shell --pure --keep GOPATH --run '<cmd>'
#   note the single quotes
#
# =====================================================================
with import <nixpkgs> {};
pkgs.mkShell {
  buildInputs = [ 

    # bazel and it's unexpressed dependencies
    bazel # may require upstream change to fix an invalid python reference inside a generated script
    stdenv.cc.cc.lib
    glibc
    python2

    # our scripts
    python3
    bash

    # go
    go

    # dep and required tooling
    dep
    git
    less # needed by git for some commands
    # dep needs this for our 1? hg dependency
    # see https://github.com/golang/dep/issues/1643
    # (23)  ✗   k8s.io/apiserver@release-1.12 wants missing rev 75cd24fc2f2c of bitbucket.org/ww/goautoneg
    mercurial

    # additional useful/basic tooling
    vim
    man

    #debugging
    #file
    #which
  ];
  LD_LIBRARY_PATH = "${stdenv.cc.cc.lib}/lib64";
  # rules_go uses a broken under nix method of discovering your go sdk
  # if GOROOT isn't exported it attempts to use a downloaded copy of go to
  # run go env which breaks on dynamic linking
  GOROOT = "${go}/share/go";
  shellHook = ''
    # leave a working terminal
    if [ -n "$IN_NIX_SHELL" ]; then
      export TERMINFO=/run/current-system/sw/share/terminfo

      # Reload terminfo
      real_TERM=$TERM; TERM=xterm; TERM=$real_TERM; unset real_TERM

      # The command bdist_wheel reads the SOURCE_DATE_EPOCH environment variable,
      # which nix-shell sets to 1. Unsetting this variable or giving it a value
      # corresponding to 1980 or later enables building wheels. -python
      unset SOURCE_DATE_EPOCH
    fi

    function setup() {
        touch user.bazelrc
        # teach bazel where python is
        echo 'build --python_path=${python2}/bin/python' >> user.bazelrc
        echo 'test --python_path=${python2}/bin/python' >> user.bazelrc

        # manually edit workspace (select doesn't work with bazel macros)
        # possibly could be done with some sort of wrapper macro instead
        git update-index --assume-unchanged WORKSPACE
        sed -i -e 's/go_register_toolchains()/go_register_toolchains(go_version = "host")/g' WORKSPACE
    }

    function teardown() {
        sed -i '/build --python_path=*/d' user.bazelrc
        sed -i '/test --python_path=*/d' user.bazelrc

        sed -i -e 's/go_register_toolchains(go_version = "host")/go_register_toolchains()/g' WORKSPACE
        git update-index --no-assume-unchanged WORKSPACE

        if [ ! -s user.bazelrc ] ; then
          rm user.bazelrc
        fi
    }

    setup
    trap teardown EXIT
  '';
}
