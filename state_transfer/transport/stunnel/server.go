package stunnel

import (
	"bytes"
	"context"
	"strconv"
	"text/template"

	"k8s.io/apimachinery/pkg/types"

	"github.com/konveyor/crane-lib/state_transfer/endpoint"

	"github.com/konveyor/crane-lib/state_transfer/transport"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stunnelServerConfTemplate = `foreground = yes
pid =
socket = l:TCP_NODELAY=1
socket = r:TCP_NODELAY=1
debug = 7
sslVersion = TLSv1.2
[rsync]
accept = {{ $.stunnelPort }}
connect = {{ $.transferPort }}
key = /etc/stunnel/certs/tls.key
cert = /etc/stunnel/certs/tls.crt
TIMEOUTclose = 0
`
)

func (s *StunnelTransport) CreateServer(c client.Client, e endpoint.Endpoint) error {
	err := createStunnelServerResources(c, s, e)
	return err
}

func createStunnelServerResources(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	s.port = stunnelPort

	err := createStunnelServerConfig(c, e)
	if err != nil {
		return err
	}

	err = createStunnelServerSecret(c, s, e)
	if err != nil {
		return err
	}

	createStunnelServerContainers(s, e.NamespacedName())

	createStunnelServerVolumes(s, e.NamespacedName())

	return nil
}

func createStunnelServerConfig(c client.Client, e endpoint.Endpoint) error {
	ports := map[string]string{
		"stunnelPort":  strconv.Itoa(int(stunnelPort)),
		"transferPort": strconv.Itoa(int(e.Port())),
	}

	var stunnelConf bytes.Buffer
	stunnelConfTemplate, err := template.New("config").Parse(stunnelServerConfTemplate)
	if err != nil {
		return err
	}

	err = stunnelConfTemplate.Execute(&stunnelConf, ports)
	if err != nil {
		return err
	}

	stunnelConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.NamespacedName().Namespace,
			Name:      stunnelConfigPrefix + e.NamespacedName().Name,
			Labels:    e.Labels(),
		},
		Data: map[string]string{
			"stunnel.conf": string(stunnelConf.Bytes()),
		},
	}

	return c.Create(context.TODO(), stunnelConfigMap, &client.CreateOptions{})

}

func getServerConfig(c client.Client, obj types.NamespacedName) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelConfigPrefix + obj.Name,
	}, cm)
	return cm, err
}

func createStunnelServerSecret(c client.Client, s *StunnelTransport, e endpoint.Endpoint) error {
	_, crt, key, err := transport.GenerateSSLCert()
	s.key = key
	s.crt = crt
	if err != nil {
		return err
	}

	stunnelSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.NamespacedName().Namespace,
			Name:      stunnelSecretPrefix + e.NamespacedName().Name,
			Labels:    e.Labels(),
		},
		Data: map[string][]byte{
			"tls.crt": s.Crt().Bytes(),
			"tls.key": s.Key().Bytes(),
		},
	}

	return c.Create(context.TODO(), stunnelSecret, &client.CreateOptions{})
}

func getServerSecret(c client.Client, obj types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := c.Get(context.Background(), types.NamespacedName{
		Namespace: obj.Namespace,
		Name:      stunnelSecretPrefix + obj.Name,
	}, secret)
	return secret, err
}

func createStunnelServerContainers(s *StunnelTransport, obj types.NamespacedName) {
	s.serverContainers = []corev1.Container{
		{
			Name:  "stunnel",
			Image: stunnelImage,
			Command: []string{
				"/bin/stunnel",
				"/etc/stunnel/stunnel.conf",
			},
			Ports: []corev1.ContainerPort{
				{
					Name:          "stunnel",
					Protocol:      corev1.ProtocolTCP,
					ContainerPort: stunnelPort,
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      stunnelConfigPrefix + obj.Name,
					MountPath: "/etc/stunnel/stunnel.conf",
					SubPath:   "stunnel.conf",
				},
				{
					Name:      stunnelSecretPrefix + obj.Name,
					MountPath: "/etc/stunnel/certs",
				},
			},
		},
	}
}

func createStunnelServerVolumes(s *StunnelTransport, obj types.NamespacedName) {
	s.serverVolumes = []corev1.Volume{
		{
			Name: stunnelConfigPrefix + obj.Name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: stunnelConfigPrefix + obj.Name,
					},
				},
			},
		},
		{
			Name: stunnelSecretPrefix + obj.Name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: stunnelSecretPrefix + obj.Name,
					Items: []corev1.KeyToPath{
						{
							Key:  "tls.crt",
							Path: "tls.crt",
						},
						{
							Key:  "tls.key",
							Path: "tls.key",
						},
					},
				},
			},
		},
	}
}