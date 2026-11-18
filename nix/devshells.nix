{ pkgs }:
let
  base = with pkgs; [
    go_latest
    gopls
    gotools
    delve
    golangci-lint
    gofumpt
    goreleaser
    gnumake
    git
    nixfmt
    statix
    deadnix
    nil
    taplo
    jq
    typos
    actionlint
    zizmor
  ];
  hook = ''
    # Use the Go from Nix; never download another toolchain.
    export GOTOOLCHAIN=local
  '';
in
{
  # Daily work: build, lint, unit and testscript tests.
  default = pkgs.mkShell {
    packages = base;
    shellHook = hook;
  };

  # make test-integration: adds the language toolchains the generators target.
  integration = pkgs.mkShell {
    packages =
      base
      ++ (with pkgs; [
        cargo
        rustc
        nodejs
        pnpm
        yarn
        bun
        jdk21
        gradle
        maven
        dotnet-sdk
        cmake
        meson
        ninja
        pkg-config
        uv
        python3
        ghc
        cabal-install
      ]);
    shellHook = hook;
  };
}
