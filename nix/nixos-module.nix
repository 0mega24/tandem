self:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.tandem;
  toml = pkgs.formats.toml { };
in
{
  options.programs.tandem = {
    enable = lib.mkEnableOption "Tandem, a multi-repo worktree manager";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "tandem.packages.\${system}.default";
      description = "The tandem package.";
    };

    plugins = lib.mkOption {
      type = lib.types.listOf lib.types.package;
      default = [ ];
      description = "External generators (tdm-gen-<id>) to put on tdm's PATH.";
    };

    settings = lib.mkOption {
      inherit (toml) type;
      default = { };
      description = "System-wide defaults, written to /etc/xdg/tandem/config.toml. A user config overrides them.";
    };
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [ (cfg.package.override { inherit (cfg) plugins; }) ];
    environment.etc."xdg/tandem/config.toml" = lib.mkIf (cfg.settings != { }) {
      source = toml.generate "tandem-config.toml" cfg.settings;
    };
  };
}
