{
  description = "ssh3d development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
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
      in
      {
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
            export LIBGL_DRIVERS_PATH="${pkgs.mesa}/lib/dri"
            export __EGL_VENDOR_LIBRARY_FILENAMES="${pkgs.mesa}/share/glvnd/egl_vendor.d/50_mesa.json"
            # SDL2 dlopen()s libEGL/libGLESv2 by soname, so they are not in its
            # RUNPATH and have to be findable at run time.
            export LD_LIBRARY_PATH="${pkgs.libGL}/lib''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
          '';
        };
      });
}
