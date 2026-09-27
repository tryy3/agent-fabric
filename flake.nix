{
  description = "agent-fabric development shell";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
    in
    {
      devShells.${system}.default = pkgs.mkShell {
        packages = with pkgs; [
          go
          gopls
          flutter
          chromium
          sqlc
          goose
          postgresql
          nodejs_22
        ];
        shellHook = ''
          export CHROME_EXECUTABLE="${pkgs.chromium}/bin/chromium"
          export npm_config_prefix="$PWD/.npm-global"
          export PATH="$npm_config_prefix/bin:$PATH"
          if ! command -v prompt-scrub >/dev/null 2>&1; then
            npm install -g @nanocollective/prompt-scrub >/dev/null 2>&1 || true
          fi
          # npm installs a symlink; prompt-scrub only runs when argv[1] is the
          # real module path, so point PROMPT_SCRUB_BIN at the resolved target.
          if command -v prompt-scrub >/dev/null 2>&1; then
            export PROMPT_SCRUB_BIN="''${PROMPT_SCRUB_BIN:-$(readlink -f "$(command -v prompt-scrub)")}"
          else
            export PROMPT_SCRUB_BIN="''${PROMPT_SCRUB_BIN:-prompt-scrub}"
          fi
        '';
      };
    };
}
