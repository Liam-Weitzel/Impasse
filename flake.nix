{
  description = "Impasse, a competitive tick based maze game played over SSH";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      # The NixOS module is not per system, so it sits outside eachDefaultSystem.
      moduleOutputs = {
        nixosModules.default = import ./nix/module.nix self;
        nixosModules.impasse = self.nixosModules.default;
      };
    in
    moduleOutputs // flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };

        # Distro/nixpkgs SDL2 builds don't ship the "offscreen" video driver,
        # which this project needs for headless (server-side) GL rendering.
        # Build it from source with that driver enabled, same as upstream's README.
        sdl2-offscreen = pkgs.stdenv.mkDerivation rec {
          pname = "SDL2-offscreen";
          version = "2.30.11";

          src = pkgs.fetchurl {
            url = "https://github.com/libsdl-org/SDL/releases/download/release-${version}/SDL2-${version}.tar.gz";
            hash = "sha256-i41K7yA4Uz2oFJZSIPiPd9YN+g8yaF+A6tZeUBM32n8=";
          };

          nativeBuildInputs = [ pkgs.pkg-config ];
          buildInputs = [ pkgs.libGL ];

          configureFlags = [ "--enable-video-offscreen=yes" ];
          enableParallelBuilding = true;
        };

        # SDL2 dlopen()s libEGL and libGLESv2 by soname, so they are not in any
        # RUNPATH and have to be findable at run time. In the dev shell that is
        # LD_LIBRARY_PATH; in the built package it is baked into a wrapper, so
        # the renderer does not depend on the environment it is started from.
        glRuntime = {
          LD_LIBRARY_PATH = "${pkgs.libGL}/lib";
          LIBGL_DRIVERS_PATH = "${pkgs.mesa}/lib/dri";
          __EGL_VENDOR_LIBRARY_FILENAMES =
            "${pkgs.mesa}/share/glvnd/egl_vendor.d/50_mesa.json";
        };

        impasse = pkgs.buildGoModule {
          pname = "impasse";
          version = "0.1.0";

          src = ./.;
          vendorHash = "sha256-Rd7pymfC0fo87VGZITAolmt9/6sNAQAlwywaPEcl/cw=";

          nativeBuildInputs = [ pkgs.pkg-config pkgs.makeWrapper ];
          buildInputs = [ sdl2-offscreen pkgs.libGL pkgs.libx11 ];

          subPackages = [ "cmd/impasse-server" "cmd/impasse-client" ];

          # The renderer needs a GPU and a pty, so its tests cannot run in the
          # sandbox. Everything else runs under `nix flake check`.
          doCheck = false;

          postFixup = ''
            wrapProgram $out/bin/impasse-client \
              --set LD_LIBRARY_PATH ${glRuntime.LD_LIBRARY_PATH} \
              --set LIBGL_DRIVERS_PATH ${glRuntime.LIBGL_DRIVERS_PATH} \
              --set __EGL_VENDOR_LIBRARY_FILENAMES ${glRuntime.__EGL_VENDOR_LIBRARY_FILENAMES}

            # The server spawns the renderer, so point it at the wrapped one
            # rather than relying on PATH. Added before the caller's arguments,
            # so passing --renderer still overrides it.
            wrapProgram $out/bin/impasse-server \
              --add-flags "--renderer $out/bin/impasse-client"
          '';

          meta = {
            description = "Competitive tick based maze game played over SSH";
            mainProgram = "impasse-server";
          };
        };
      in
      {
        packages.default = impasse;
        packages.impasse = impasse;

        apps.default = {
          type = "app";
          program = "${impasse}/bin/impasse-server";
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [
            pkgs.go
            pkgs.gotools
            pkgs.gopls
            pkgs.pkg-config
            sdl2-offscreen
            pkgs.libGL
            pkgs.mesa
            pkgs.libx11
          ];

          PKG_CONFIG_PATH = "${sdl2-offscreen}/lib/pkgconfig";

          shellHook = ''
            export LIBGL_DRIVERS_PATH="${glRuntime.LIBGL_DRIVERS_PATH}"
            export __EGL_VENDOR_LIBRARY_FILENAMES="${glRuntime.__EGL_VENDOR_LIBRARY_FILENAMES}"
            export LD_LIBRARY_PATH="${glRuntime.LD_LIBRARY_PATH}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
          '';
        };
      });
}
