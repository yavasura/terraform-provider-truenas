# Create a custom Docker Compose app
resource "truenas_app" "nginx" {
  name       = "nginx"
  custom_app = true

  custom_compose_config_string = <<-EOT
    services:
      nginx:
        image: nginx:latest
        ports:
          - "8080:80"
        volumes:
          - /mnt/tank/apps/nginx/config:/etc/nginx/conf.d
          - /mnt/tank/apps/nginx/html:/usr/share/nginx/html
        restart: unless-stopped
  EOT
}

# Output the app state
output "nginx_state" {
  value = truenas_app.nginx.state
}
