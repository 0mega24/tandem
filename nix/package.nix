{
  lib,
  stdenv,
  buildGoLatestModule,
  git,
  installShellFiles,
  makeWrapper,
  version,
  rev ? "unknown",
  # External generators (tdm-gen-<id> executables) to put on tdm's PATH.
  plugins ? [ ],
}:

buildGoLatestModule (finalAttrs: {
  pname = "tandem";
  inherit version;

  # Only the files the build needs, so a docs change does not rebuild tandem.
  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../cmd
      ../internal
      (lib.fileset.maybeMissing ../schema)
    ];
  };

  # Run `make nix-vendor-hash` after a go.mod change.
  vendorHash = "sha256-4ri6XREY8fSY2tWwBNuGtH4nd1y5CaHH9xSQEO66+OQ=";

  # Generated files are committed, so the build does not run go generate.
  # No subPackages: with it, checkPhase runs only the cmd/tdm tests. tools/* are dev-only mains.
  excludedPackages = [ "tools" ];
  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X github.com/0mega24/tandem/internal/version.Version=${finalAttrs.version}"
    "-X github.com/0mega24/tandem/internal/version.Commit=${rev}"
  ];

  nativeBuildInputs = [
    installShellFiles
    makeWrapper
  ];

  # Unit, golden and testscript tests need git, not language toolchains.
  nativeCheckInputs = [ git ];
  preCheck = ''
    export HOME="$TMPDIR"
    git config --global user.name nix-build
    git config --global user.email nix-build@example.invalid
    git config --global init.defaultBranch main
  '';
  checkFlags = [ "-skip=Integration" ];

  postInstall =
    lib.optionalString (stdenv.buildPlatform.canExecute stdenv.hostPlatform) ''
      installShellCompletion --cmd tdm \
        --bash <($out/bin/tdm completion bash) \
        --zsh <($out/bin/tdm completion zsh) \
        --fish <($out/bin/tdm completion fish)
    ''
    + ''
      # The user's own git comes first; Nix git is the fallback.
      wrapProgram $out/bin/tdm --suffix PATH : ${lib.makeBinPath ([ git ] ++ plugins)}
    '';

  meta = {
    description = "One branch across many repos, as git worktrees, with language workspace files";
    homepage = "https://github.com/0mega24/tandem";
    license = lib.licenses.mit;
    mainProgram = "tdm";
    platforms = lib.platforms.unix;
  };
})
