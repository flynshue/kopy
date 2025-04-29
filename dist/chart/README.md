# kopy

![Version: v0.0.2](https://img.shields.io/badge/Version-v0.0.2-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.0.2](https://img.shields.io/badge/AppVersion-v0.0.2-informational?style=flat-square)

A Helm chart to distribute the project kopy

**Homepage:** <https://github.com/flynshue/kopy>

## Source Code

* <https://github.com/flynshue/kopy>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| certmanager.enable | bool | `false` |  |
| controllerManager.container.args[0] | string | `"--leader-elect"` |  |
| controllerManager.container.args[1] | string | `"--metrics-bind-address=:8443"` |  |
| controllerManager.container.args[2] | string | `"--health-probe-bind-address=:8081"` |  |
| controllerManager.container.image.repository | string | `"ghcr.io/flynshue/kopy"` |  |
| controllerManager.container.image.tag | string | `"v0.0.4"` |  |
| controllerManager.container.livenessProbe.httpGet.path | string | `"/healthz"` |  |
| controllerManager.container.livenessProbe.httpGet.port | int | `8081` |  |
| controllerManager.container.livenessProbe.initialDelaySeconds | int | `15` |  |
| controllerManager.container.livenessProbe.periodSeconds | int | `20` |  |
| controllerManager.container.readinessProbe.httpGet.path | string | `"/readyz"` |  |
| controllerManager.container.readinessProbe.httpGet.port | int | `8081` |  |
| controllerManager.container.readinessProbe.initialDelaySeconds | int | `5` |  |
| controllerManager.container.readinessProbe.periodSeconds | int | `10` |  |
| controllerManager.container.resources.limits.cpu | string | `"500m"` |  |
| controllerManager.container.resources.limits.memory | string | `"128Mi"` |  |
| controllerManager.container.resources.requests.cpu | string | `"10m"` |  |
| controllerManager.container.resources.requests.memory | string | `"64Mi"` |  |
| controllerManager.container.securityContext.allowPrivilegeEscalation | bool | `false` |  |
| controllerManager.container.securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| controllerManager.replicas | int | `1` |  |
| controllerManager.securityContext.runAsNonRoot | bool | `true` |  |
| controllerManager.securityContext.seccompProfile.type | string | `"RuntimeDefault"` |  |
| controllerManager.serviceAccountName | string | `"kopy-controller-manager"` |  |
| controllerManager.terminationGracePeriodSeconds | int | `10` |  |
| crd.enable | bool | `true` |  |
| crd.keep | bool | `true` |  |
| metrics.enable | bool | `true` |  |
| networkPolicy.enable | bool | `false` |  |
| prometheus.enable | bool | `false` |  |
| rbac.enable | bool | `true` |  |
