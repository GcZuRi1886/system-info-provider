{
  description = "Go project built with Nix flakes";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in {
        packages.default = pkgs.buildGoModule {
          pname = "system-info-provider";
          version = "0.3.0";

          # Path to your main.go or module root
          src = ./.;

          vendorHash = "sha256-mqui9KN6LUUghjP/sgp09QKMIxWBrtwlM6gNkA+r7lc=";
        };

        # Optional: create a dev environment with Go tools
        devShells.default = pkgs.mkShell {
          buildInputs = [
            pkgs.go
            pkgs.gopls
            pkgs.go-tools
          ];
        };
      }
    );
}

