package server_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ContainerCreate", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	newContainerConfig := func() *types.ContainerConfig {
		return &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{},
			Image:    &types.ImageSpec{},
			Linux: &types.LinuxContainerConfig{
				Resources: &types.LinuxContainerResources{},
				SecurityContext: &types.LinuxContainerSecurityContext{
					Capabilities:     &types.Capability{},
					NamespaceOptions: &types.NamespaceOption{},
					SelinuxOptions:   &types.SELinuxOption{},
					RunAsUser:        &types.Int64Value{},
					RunAsGroup:       &types.Int64Value{},
				},
			},
		}
	}

	newPodSandboxConfig := func() *types.PodSandboxConfig {
		return &types.PodSandboxConfig{
			Metadata:     &types.PodSandboxMetadata{},
			DnsConfig:    &types.DNSConfig{},
			PortMappings: []*types.PortMapping{},
			Linux: &types.LinuxPodSandboxConfig{
				SecurityContext: &types.LinuxSandboxSecurityContext{
					NamespaceOptions: &types.NamespaceOption{},
					SelinuxOptions:   &types.SELinuxOption{},
					RunAsUser:        &types.Int64Value{},
					RunAsGroup:       &types.Int64Value{},
				},
			},
		}
	}

	t.Describe("ContainerCreate:AllValues", func() {
		It("should not segfault if field in config is empty", func() {
			// Given
			addContainerAndSandbox()

			containerConfig := &types.ContainerConfig{}
			v := reflect.ValueOf(containerConfig).Elem()
			v = setToZero(v)

			_ = &types.CreateContainerRequest{
				PodSandboxId: testSandbox.ID(),
				Config:       containerConfig,
			}

		})
	})

	t.Describe("ContainerCreate", func() {
		It("should fail when container config image is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId: testSandbox.ID(),
					Config: &types.ContainerConfig{
						Metadata: &types.ContainerMetadata{
							Name: "name",
						},
					},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata name is empty", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId: testSandbox.ID(),
					Config: &types.ContainerConfig{
						Metadata: &types.ContainerMetadata{},
					},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId: testSandbox.ID(),
					Config:       &types.ContainerConfig{},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container is stopped", func() {
			// Given
			addContainerAndSandbox()
			testSandbox.SetStopped(false)

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when sandbox not found", func() {
			// Given
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(BeNil())

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on invalid pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxId:  testSandbox.ID(),
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on empty pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					Config:        newContainerConfig(),
					SandboxConfig: newPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})

// func getZero(v reflect.Value) reflect.Value {
// 	switch v.Kind() {
// 	case reflect.Func:
// 		panic("unexpected!")
// 	case reflect.Map:
// 		return make(map[interface{}]interface{})
// 	case reflect.Slice:
// 		return make([]interface{})
// 	case reflect.Array:
// 		panic("unexpected!")
// 	case reflect.Struct:
// 		return nil
// 	}
// 	// Compare other types directly:
// 	return reflect.Zero(v.Type())
// }
// func setToZero(v reflect.Value) reflect.Value {
// 	switch v.Kind() {
// 	case reflect.Struct:
// 		for i := 0; i < v.NumField(); i++ {
// 			fmt.Println("calling set on", v.Type().Field(i).Name, "type", v.Field(i).Kind())
// 			v.Field(i).Set(setToZero(v.Field(i)))
// 		}
// 		return v
// 	case reflect.Ptr:
// 		newV := reflect.New(v.Type().Elem())
// 		setToZero(newV)
// 		return newV.Elem()
// 	}
// 	newV := reflect.New(v.Type()).Elem()
// 	fmt.Println("going from", v.Interface(), "to", newV.Interface())
// 	return newV
// }
func setToZero(v reflect.Value) {
	// fmt.Println("calling set on", v.Type().Field(i).Name, "type", v.Field(i).Kind(), "value" v.Interface())
	switch v.Kind() {
	case reflect.Ptr:
		v.Set(reflect.New(v.Type().Elem()))
		setToZero(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			setToZero(v.Field(i))
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	case reflect.Int:
		v.SetInt(0)
	case reflect.String:
		v.SetString("")
	}
	panic("unset type")
}
