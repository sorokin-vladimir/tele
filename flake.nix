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
        # Derived from the checked-out commit rather than hardcoded, so it
        # can't drift from what's actually built (see PR review discussion).
        version = self.shortRev or self.dirtyShortRev or "unknown";
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "tele";
          inherit version;

          src = ./.;

          # Must be regenerated whenever go.mod/go.sum changes: run `nix build`,
          # copy the "got: sha256-..." hash it reports, and paste it here.
          vendorHash = "sha256-LcJyKDgvKj1hrDAO0ukpjb4CX3O3qyHQ4RssTcdmNO0=";

          subPackages = [ "cmd/tele" ];

          env.CGO_ENABLED = "0";

          # main.buildAPIID / main.buildAPIHash / main.appName are release-time
          # secrets/channel flags injected by .goreleaser.yaml — deliberately
          # left unset here. A build from source falls back to the app key in
          # internal/appkey, so this binary reaches the login screen without the
          # person registering an application; a key in config.yml outranks it.
          #
          # The version symbol lives in internal/version, not in main: the
          # linker silently ignores -X for a name that does not exist, so the
          # wrong path here costs nothing at build time and reports "dev"
          # forever. cmd/tele's TestHomebrewCoreContract pins the right one.
          ldflags = [
            "-s"
            "-w"
            "-X github.com/sorokin-vladimir/tele/internal/version.Version=${version}"
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
