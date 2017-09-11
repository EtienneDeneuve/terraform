package azurerm

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/containerinstance"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
)

func resourceArmContainerGroup() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmContainerGroupCreate,
		Read:   resourceArmContainerGroupRead,
		Delete: resourceArmContainerGroupDelete,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"location": locationSchema(),

			"resource_group_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"ip_address_type": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
			},

			"os_type": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: ignoreCaseDiffSuppressFunc,
			},

			"tags": tagsForceNewSchema(),

			"ip_address": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"container": {
				Type:     schema.TypeList,
				Required: true,
				ForceNew: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{

						"name": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},

						"image": {
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},

						"cpu": {
							Type:     schema.TypeFloat,
							Required: true,
							ForceNew: true,
						},

						"memory": {
							Type:     schema.TypeFloat,
							Required: true,
							ForceNew: true,
						},

						"port": {
							Type:         schema.TypeInt,
							Optional:     true,
							ForceNew:     true,
							ValidateFunc: validation.IntBetween(1, 65535),
						},

						"protocol": {
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
							ValidateFunc: validation.StringInSlice([]string{
								"tcp",
								"udp",
							}, true),
						},

						"env_var": {
							Type:     schema.TypeList,
							Optional: true,
							ForceNew: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"name": {
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},

									"value": {
										Type:     schema.TypeString,
										Required: true,
										ForceNew: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func resourceArmContainerGroupCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient)
	containerGroupsClient := client.containerGroupsClient

	// container group properties
	resGroup := d.Get("resource_group_name").(string)
	name := d.Get("name").(string)
	location := d.Get("location").(string)
	OSType := d.Get("os_type").(string)
	IPAddressType := d.Get("ip_address_type").(string)
	tags := d.Get("tags").(map[string]interface{})

	containersConfig := d.Get("container").([]interface{})
	containers := make([]containerinstance.Container, 0, len(containersConfig))
	containerGroupPorts := make([]containerinstance.Port, 0, len(containersConfig))

	for _, containerConfig := range containersConfig {
		data := containerConfig.(map[string]interface{})

		// required
		name := data["name"].(string)
		image := data["image"].(string)
		cpu := data["cpu"].(float64)
		memory := data["memory"].(float64)

		// optional
		port := int32(data["port"].(int))
		protocol := data["protocol"].(string)

		container := containerinstance.Container{
			Name: &name,
			ContainerProperties: &containerinstance.ContainerProperties{
				Image: &image,
				Resources: &containerinstance.ResourceRequirements{
					Requests: &containerinstance.ResourceRequests{
						MemoryInGB: &memory,
						CPU:        &cpu,
					},
				},
			},
		}

		if port != 0 {
			// container port (port number)
			containerPort := containerinstance.ContainerPort{
				Port: &port,
			}

			container.Ports = &[]containerinstance.ContainerPort{containerPort}

			// container group port (port number + protocol)
			containerGroupPort := containerinstance.Port{
				Port: &port,
			}

			if protocol != "" {
				containerGroupPort.Protocol = containerinstance.ContainerGroupNetworkProtocol(strings.ToUpper(protocol))
			}

			containerGroupPorts = append(containerGroupPorts, containerGroupPort)
		}

		// envVars := data["env_vars"].([]interface{})
		// if len(envVars) > 0 {
		// 	envVarsList := make([]containerinstance.EnvironmentVariable, 0, len(envVars))

		// 	for _, envVarRaw := range envVars {
		// 		envVar := containerinstance.EnvironmentVariable{
		// 			Name:  &envVarRaw.get("name").(string),
		// 			Value: &envVarRaw.get("value").(string),
		// 		}
		// 		envVarsList = append(envVarsList, envVar)
		// 	}
		// 	container.EnvironmentVariables = &envVarsList
		// }

		containers = append(containers, container)
	}

	containerGroup := containerinstance.ContainerGroup{
		Name:     &name,
		Location: &location,
		Tags:     expandTags(tags),
		ContainerGroupProperties: &containerinstance.ContainerGroupProperties{
			Containers: &containers,
			IPAddress: &containerinstance.IPAddress{
				Type:  &IPAddressType,
				Ports: &containerGroupPorts,
			},
			OsType: containerinstance.OperatingSystemTypes(OSType),
		},
	}

	_, error := containerGroupsClient.CreateOrUpdate(resGroup, name, containerGroup)
	if error != nil {
		return error
	}

	read, readErr := containerGroupsClient.Get(resGroup, name)
	if readErr != nil {
		return readErr
	}

	if read.ID == nil {
		return fmt.Errorf("Cannot read container group %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*read.ID)

	return resourceArmContainerGroupRead(d, meta)
}
func resourceArmContainerGroupRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient)
	containterGroupsClient := client.containerGroupsClient

	id, err := parseAzureResourceID(d.Id())

	if err != nil {
		return err
	}

	resGroup := id.ResourceGroup
	name := id.Path["containerGroups"]

	resp, error := containterGroupsClient.Get(resGroup, name)

	if error != nil {
		return error
	}

	d.Set("name", name)
	d.Set("resource_group_name", resGroup)
	d.Set("location", azureRMNormalizeLocation(*resp.Location))
	flattenAndSetTags(d, resp.Tags)

	d.Set("os_type", string(resp.OsType))
	d.Set("ip_address_type", *resp.IPAddress.Type)
	d.Set("ip_address", *resp.IPAddress.IP)

	containers := *resp.Containers

	containerConfigs := make([]interface{}, 0, len(containers))
	for _, container := range containers {
		containerConfig := make(map[string]interface{})
		containerConfig["name"] = *container.Name
		containerConfig["image"] = *container.Image

		resourceRequests := *(*container.Resources).Requests
		containerConfig["cpu"] = *resourceRequests.CPU
		containerConfig["memory"] = *resourceRequests.MemoryInGB

		if len(*container.Ports) > 0 {
			containerConfig["port"] = *(*container.Ports)[0].Port
		}
		// protocol isn't returned in container config

		containerConfigs = append(containerConfigs, containerConfig)
	}

	d.Set("container", containerConfigs)

	return nil
}
func resourceArmContainerGroupDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*ArmClient)
	containterGroupsClient := client.containerGroupsClient

	// container group properties
	resGroup := d.Get("resource_group_name").(string)
	name := d.Get("name").(string)

	_, error := containterGroupsClient.Delete(resGroup, name)

	if error != nil {
		return error
	}

	return nil
}
