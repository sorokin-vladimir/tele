{
  description = "tele — a terminal-native Telegram client built for keyboard-driven workflows";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        version = "1.8.0";
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "tele";
          inherit version;

          src = ./.;

          vendorHash = "sha256-tZkIsry/MyILMb2USafVmzBfTbqeNQNZ/QtRryGCHgQ=";

          subPackages = [ "cmd/tele" ];

          env.CGO_ENABLED = "0";

          # main.buildAPIID / main.buildAPIHash / main.appName are release-time
          # secrets/channel flags injected by .goreleaser.yaml — deliberately
          # left unset here. Users supply real Telegram credentials via
          # config.yml (see config.yml.example), which main.go already
          # supports as a fallback.
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${version}"
          ];

          doCheck = true;

          meta = {
            description = "A terminal-native Telegram client built for keyboard-driven workflows";
            homepage = "https://github.com/sorokin-vladimir/tele";
            license = pkgs.lib.licenses.gpl3Only;
            mainProgram = "tele";
            platforms = pkgs.lib.platforms.unix;
          };
        };

        apps.default = flake-utils.lib.mkApp { drv = self.packages.${system}.default; };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            golangci-lint
            lefthook
            gotools
            xclip
            wl-clipboard
          ];

          shellHook = ''
            echo "tele dev shell — go $(go version | cut -d' ' -f3)"
            echo "  go run ./cmd/tele/ -config .config/tele/config.yml   # run (dev config)"
            echo "  go test ./...                                        # test"
            echo "  golangci-lint run ./...                              # lint"
          '';
        };

        formatter = pkgs.nixfmt;
      }
    );
}
