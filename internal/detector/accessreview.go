package detector

import (
	"context"
	"fmt"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"secha/internal/experiment"
	"secha/internal/k8s"
)

func init() {
	Register("k8s-access-review", newAccessReviewDetector)
}

const accessReviewPollInterval = 250 * time.Millisecond

type accessReviewConfig struct {
	Subject struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"subject"`
	Resource struct {
		Namespace string `json:"namespace"`
		Verb      string `json:"verb"`
		Resource  string `json:"resource"`
		Name      string `json:"name"`
	} `json:"resource"`
}

type accessReviewDetector struct {
	client    *k8s.Client
	user      string
	namespace string
	verb      string
	resource  string
	name      string
}

func newAccessReviewDetector(spec experiment.Spec) (Detector, error) {
	var cfg accessReviewConfig
	if err := spec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode k8s-access-review detector: %w", err)
	}
	if cfg.Subject.Namespace == "" || cfg.Subject.Name == "" {
		return nil, fmt.Errorf("k8s-access-review: subject.namespace and subject.name are required")
	}
	if cfg.Resource.Verb == "" || cfg.Resource.Resource == "" {
		return nil, fmt.Errorf("k8s-access-review: resource.verb and resource.resource are required")
	}
	client, err := k8s.NewClient()
	if err != nil {
		return nil, err
	}
	return &accessReviewDetector{
		client:    client,
		user:      fmt.Sprintf("system:serviceaccount:%s:%s", cfg.Subject.Namespace, cfg.Subject.Name),
		namespace: cfg.Resource.Namespace,
		verb:      cfg.Resource.Verb,
		resource:  cfg.Resource.Resource,
		name:      cfg.Resource.Name,
	}, nil
}

func (d *accessReviewDetector) allowed(ctx context.Context) (bool, error) {
	sar := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User: d.user,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: d.namespace,
				Verb:      d.verb,
				Resource:  d.resource,
				Name:      d.name,
			},
		},
	}
	res, err := d.client.Clientset.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("subjectaccessreview for %s: %w", d.user, err)
	}
	return res.Status.Allowed, nil
}

func (d *accessReviewDetector) Observe(ctx context.Context, window time.Duration) (Signal, error) {
	deadline := time.Now().Add(window)
	for {
		allowed, err := d.allowed(ctx)
		if err != nil {
			return Signal{}, err
		}
		if allowed {
			return Signal{
				Detected: true,
				At:       time.Now(),
				Source:   "k8s-access-review",
				Detail:   fmt.Sprintf("%s is authorized to %s %s in %s", d.user, d.verb, d.resource, d.namespace),
			}, nil
		}
		if time.Now().After(deadline) {
			return Signal{Detected: false, Source: "k8s-access-review"}, nil
		}
		select {
		case <-ctx.Done():
			return Signal{}, ctx.Err()
		case <-time.After(accessReviewPollInterval):
		}
	}
}

func (d *accessReviewDetector) Describe() string {
	return fmt.Sprintf("access review: can %s %s %s in %s", d.user, d.verb, d.resource, d.namespace)
}
