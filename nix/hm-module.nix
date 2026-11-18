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
  finalPackage = cfg.package.override { inherit (cfg) plugins; };
  exe = lib.getExe finalPackage;
  posixHelper = ''
    tcd() { cd "$(${exe} path "$1")"; }
  '';
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
      example = lib.literalExpression ''
        {
          discovery.root_dir = "~/repos";
          spaces.root_dir = "~/workspaces";
          git.branch_prefix = "me/";
          generate.rust.mode = "auto";
        }
      '';
      description = "Written to $XDG_CONFIG_HOME/tandem/config.toml.";
    };

    shellHelper = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Add the tcd <space> function to bash, zsh and fish.";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ finalPackage ];

    xdg.configFile."tandem/config.toml" = lib.mkIf (cfg.settings != { }) {
      source = toml.generate "tandem-config.toml" cfg.settings;
    };

    programs = {
      bash.initExtra = lib.mkIf cfg.shellHelper posixHelper;
      zsh.initContent = lib.mkIf cfg.shellHelper posixHelper;
      fish.functions.tcd = lib.mkIf cfg.shellHelper "cd (${exe} path $argv[1])";
    };
  };
}
