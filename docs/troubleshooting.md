# Troubleshooting

## Collecting logs from source controller manager

Retrieve pod logs from the `controller-manager` running in the `source-system` namespace.

```bash
kubectl logs -n source-system -l control-plane=controller-manager
```

For example:

```bash
2021-11-18T17:59:43.152Z	INFO	controller.imagerepository	Starting EventSource	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "ImageRepository", "source": "kind source: /, Kind="}
2021-11-18T17:59:43.152Z	INFO	controller.metarepository	Starting EventSource	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "MetaRepository", "source": "kind source: /, Kind="}
2021-11-18T17:59:43.152Z	INFO	controller.metarepository	Starting EventSource	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "MetaRepository", "source": "kind source: /, Kind="}
2021-11-18T17:59:43.152Z	INFO	controller.metarepository	Starting EventSource	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "MetaRepository", "source": "kind source: /, Kind="}
2021-11-18T17:59:43.152Z	INFO	controller.metarepository	Starting Controller	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "MetaRepository"}
2021-11-18T17:59:43.152Z	INFO	controller.imagerepository	Starting EventSource	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "ImageRepository", "source": "kind source: /, Kind="}
2021-11-18T17:59:43.152Z	INFO	controller.imagerepository	Starting EventSource	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "ImageRepository", "source": "kind source: /, Kind="}
2021-11-18T17:59:43.152Z	INFO	controller.imagerepository	Starting Controller	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "ImageRepository"}
2021-11-18T17:59:43.389Z	INFO	controller.metarepository	Starting workers	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "MetaRepository", "worker count": 1}
2021-11-18T17:59:43.391Z	INFO	controller.imagerepository	Starting workers	{"reconciler group": "source.apps.tanzu.vmware.com", "reconciler kind": "ImageRepository", "worker count": 1}
```
