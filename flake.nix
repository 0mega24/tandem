{
  description = "tandem: one branch across many repos, as git worktrees, with language workspace files";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      # One version source: internal/version/VERSION (also embedded in the Go binary).
      baseVersion = lib.fileContents ./internal/version/VERSION;
      version = "${baseVersion}+${self.shortRev or self.dirtyShortRev or "dirty"}";
      rev = self.rev or self.dirtyRev or "dirty";

      mkTandem = pkgs: pkgs.callPackage ./nix/package.nix { inherit version rev; };
    in
    {
      packages = forAllSystems (pkgs: {
        tandem = mkTandem pkgs;
        default = self.packages.${pkgs.stdenv.hostPlatform.system}.tandem;
      });

      overlays.default = final: _prev: { tandem = mkTandem final; };

      devShells = forAllSystems (pkgs: import ./nix/devshells.nix { inherit pkgs; });

      nixosModules.default = import ./nix/nixos-module.nix self;
      homeManagerModules.default = import ./nix/hm-module.nix self;

      checks = forAllSystems (pkgs: {
        # Builds tandem and runs the unit, golden and testscript tests in checkPhase.
        package = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
        nixfmt = pkgs.runCommand "nixfmt-check" { nativeBuildInputs = [ pkgs.nixfmt ]; } ''
          nixfmt --check ${./flake.nix} ${./nix}/*.nix
          touch $out
        '';
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
