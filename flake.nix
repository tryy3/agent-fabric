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
        ];
        shellHook = ''
          export CHROME_EXECUTABLE="${pkgs.chromium}/bin/chromium"
        '';
      };
    };
}
