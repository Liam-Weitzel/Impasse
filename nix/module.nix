self:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.impasse;
  pkg = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
in
{
  options.services.impasse = {
    enable = lib.mkEnableOption "the Impasse game server";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkg;
      description = "Impasse package to run.";
    };

    port = lib.mkOption {
      type = lib.types.port;
      default = 22;
      description = ''
        SSH port players connect on. Anything below 1024 needs
        lowerUnprivilegedPorts, since the service does not run as root.
      '';
    };

    botPort = lib.mkOption {
      type = lib.types.port;
      default = 2223;
      description = "TCP port the bot API listens on.";
    };

    map = lib.mkOption {
      type = lib.types.path;
      default = "${cfg.package.src}/maps/vault.txt";
      defaultText = lib.literalExpression ''"''${package.src}/maps/vault.txt"'';
      description = "ASCII map to load.";
    };

    clientIdFile = lib.mkOption {
      type = lib.types.path;
      example = "/run/secrets/impasse-github-client-id";
      description = ''
        File holding the GitHub OAuth app client id. A file rather than a
        string so it does not land in the world readable nix store. The client
        id is not a secret, but the same mechanism is what a secret would need
        and there is no reason to have two.

        The client secret is never used and must not be given.
      '';
    };

    openFirewall = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Open the SSH and bot ports.";
    };

    lowerUnprivilegedPorts = lib.mkOption {
      type = lib.types.bool;
      default = cfg.port < 1024;
      defaultText = lib.literalExpression "port < 1024";
      description = ''
        Lower the floor on ports an unprivileged process may bind, so the
        service can take port 22 without any privilege at all.

        The alternative, a CAP_NET_BIND_SERVICE ambient capability, breaks the
        renderer. A child of a privileged process execs with AT_SECURE=1, and
        glibc then ignores LD_LIBRARY_PATH when resolving libraries. SDL loads
        libEGL by dlopen, so every renderer would fail to start.

        This applies to IPv6 as well despite the ipv4 in the name.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    boot.kernel.sysctl."net.ipv4.ip_unprivileged_port_start" =
      lib.mkIf cfg.lowerUnprivilegedPorts cfg.port;

    networking.firewall.allowedTCPPorts =
      lib.mkIf cfg.openFirewall [ cfg.port cfg.botPort ];

    users.users.impasse = {
      isSystemUser = true;
      group = "impasse";
      description = "Impasse game server";
    };
    users.groups.impasse = { };

    systemd.services.impasse = {
      description = "Impasse game server";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];

      serviceConfig = {
        User = "impasse";
        Group = "impasse";
        Restart = "on-failure";

        # Scores, the host key and the renderer socket all live here.
        StateDirectory = "impasse";
        WorkingDirectory = "/var/lib/impasse";

        # systemd puts the client id in a file only this unit can read, and the
        # server reads it from there. %d is the credentials directory. Keeping
        # it in a file rather than an argument or the environment means it is
        # neither in the nix store nor on a command line every process can see.
        LoadCredential = "client-id:${cfg.clientIdFile}";

        ExecStart = lib.concatStringsSep " " [
          (lib.getExe cfg.package)
          "--github-client-id-file %d/client-id"
          "--port ${toString cfg.port}"
          "--bots :${toString cfg.botPort}"
          "--map ${cfg.map}"
          "--db /var/lib/impasse/impasse.db"
          # In the state directory, so it survives restarts and redeploys.
          # A host key that changes greets every returning player with
          # REMOTE HOST IDENTIFICATION HAS CHANGED.
          "--key /var/lib/impasse/host_key"
        ];

        NoNewPrivileges = true;
        PrivateTmp = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        ProtectKernelTunables = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];
        RestrictSUIDSGID = true;
        LockPersonality = true;

        # The renderer needs the GPU. Without this it falls back to software
        # rendering if mesa can manage it, and fails outright if it cannot.
        DeviceAllow = [ "/dev/dri rw" ];
        SupplementaryGroups = [ "video" "render" ];
      };
    };
  };
}
