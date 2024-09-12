// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package approleassignments

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/common-types/stable"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/serviceprincipals/stable/approleassignedto"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/serviceprincipals/stable/serviceprincipal"
	"github.com/hashicorp/go-azure-sdk/sdk/nullable"
	"github.com/hashicorp/terraform-provider-azuread/internal/clients"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/tf"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/tf/validation"
	"github.com/hashicorp/terraform-provider-azuread/internal/services/approleassignments/parse"
)

func appRoleAssignmentResource() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		CreateContext: appRoleAssignmentResourceCreate,
		ReadContext:   appRoleAssignmentResourceRead,
		DeleteContext: appRoleAssignmentResourceDelete,

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(5 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(5 * time.Minute),
		},

		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.AppRoleAssignmentID(id)
			return err
		}),

		Schema: map[string]*pluginsdk.Schema{
			"app_role_id": {
				Description:      "The ID of the app role to be assigned",
				Type:             pluginsdk.TypeString,
				Required:         true,
				ForceNew:         true,
				ValidateDiagFunc: validation.ValidateDiag(validation.IsUUID),
			},

			"principal_object_id": {
				Description:      "The object ID of the user, group or service principal to be assigned this app role",
				Type:             pluginsdk.TypeString,
				Required:         true,
				ForceNew:         true,
				ValidateDiagFunc: validation.ValidateDiag(validation.IsUUID),
			},

			"resource_object_id": {
				Description:      "The object ID of the service principal representing the resource",
				Type:             pluginsdk.TypeString,
				Required:         true,
				ForceNew:         true,
				ValidateDiagFunc: validation.ValidateDiag(validation.IsUUID),
			},

			"principal_display_name": {
				Description: "The display name of the principal to which the app role is assigned",
				Type:        pluginsdk.TypeString,
				Computed:    true,
			},

			"principal_type": {
				Description: "The object type of the principal to which the app role is assigned",
				Type:        pluginsdk.TypeString,
				Computed:    true,
			},

			"resource_display_name": {
				Description: "The display name of the application representing the resource",
				Type:        pluginsdk.TypeString,
				Computed:    true,
			},
		},
	}
}

func appRoleAssignmentResourceCreate(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).AppRoleAssignments.AppRoleAssignedToClient
	servicePrincipalClient := meta.(*clients.Client).AppRoleAssignments.ServicePrincipalClient

	appRoleId := d.Get("app_role_id").(string)
	principalId := d.Get("principal_object_id").(string)
	resourceId := d.Get("resource_object_id").(string)

	if resp, err := servicePrincipalClient.GetServicePrincipal(ctx, stable.NewServicePrincipalID(resourceId), serviceprincipal.DefaultGetServicePrincipalOperationOptions()); err != nil {
		if response.WasNotFound(resp.HttpResponse) {
			return tf.ErrorDiagPathF(err, "principal_object_id", "Service principal not found for resource (Object ID: %q)", resourceId)
		}
		return tf.ErrorDiagF(err, "Could not retrieve service principal for resource (Object ID: %q)", resourceId)
	}

	properties := stable.AppRoleAssignment{
		AppRoleId:   pointer.To(appRoleId),
		PrincipalId: nullable.Value(principalId),
		ResourceId:  nullable.Value(resourceId),
	}

	resp, err := client.CreateAppRoleAssignedTo(ctx, stable.NewServicePrincipalID(resourceId), properties, approleassignedto.DefaultCreateAppRoleAssignedToOperationOptions())
	if err != nil {
		return tf.ErrorDiagF(err, "Could not create app role assignment")
	}

	appRoleAssignment := resp.Model
	if appRoleAssignment == nil {
		return tf.ErrorDiagF(errors.New("model was nil"), "Could not create app role assignment")
	}

	if appRoleAssignment.Id == nil || *appRoleAssignment.Id == "" {
		return tf.ErrorDiagF(errors.New("ID returned for app role assignment is nil"), "Bad API response")
	}

	if appRoleAssignment.ResourceId.IsNull() || appRoleAssignment.ResourceId.GetOrZero() == "" {
		return tf.ErrorDiagF(errors.New("Resource ID returned for app role assignment is nil"), "Bad API response")
	}

	id := parse.NewAppRoleAssignmentID(appRoleAssignment.ResourceId.GetOrZero(), *appRoleAssignment.Id)
	d.SetId(id.String())

	return appRoleAssignmentResourceRead(ctx, d, meta)
}

func appRoleAssignmentResourceRead(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).AppRoleAssignments.AppRoleAssignedToClient

	resourceId, err := parse.AppRoleAssignmentID(d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "id", "Parsing app role assignment with ID %q", d.Id())
	}

	id := stable.NewServicePrincipalIdAppRoleAssignedToID(resourceId.ResourceId, resourceId.AssignmentId)

	resp, err := client.GetAppRoleAssignedTo(ctx, id, approleassignedto.DefaultGetAppRoleAssignedToOperationOptions())
	if err != nil {
		if response.WasNotFound(resp.HttpResponse) {
			log.Printf("[DEBUG] %s was not found - removing from state!", id)
			d.SetId("")
			return nil
		}
		return tf.ErrorDiagF(err, "retrieving %s", id)
	}

	appRoleAssignment := resp.Model
	if appRoleAssignment == nil {
		return tf.ErrorDiagF(errors.New("model was nil"), "retrieving %s", id)
	}

	tf.Set(d, "app_role_id", appRoleAssignment.AppRoleId)
	tf.Set(d, "principal_display_name", appRoleAssignment.PrincipalDisplayName.GetOrZero())
	tf.Set(d, "principal_object_id", appRoleAssignment.PrincipalId.GetOrZero())
	tf.Set(d, "principal_type", appRoleAssignment.PrincipalType.GetOrZero())
	tf.Set(d, "resource_display_name", appRoleAssignment.ResourceDisplayName.GetOrZero())
	tf.Set(d, "resource_object_id", appRoleAssignment.ResourceId.GetOrZero())

	return nil
}

func appRoleAssignmentResourceDelete(ctx context.Context, d *pluginsdk.ResourceData, meta interface{}) pluginsdk.Diagnostics {
	client := meta.(*clients.Client).AppRoleAssignments.AppRoleAssignedToClient

	resourceId, err := parse.AppRoleAssignmentID(d.Id())
	if err != nil {
		return tf.ErrorDiagPathF(err, "id", "Parsing app role assignment with ID %q", d.Id())
	}

	id := stable.NewServicePrincipalIdAppRoleAssignedToID(resourceId.ResourceId, resourceId.AssignmentId)

	if _, err = client.DeleteAppRoleAssignedTo(ctx, id, approleassignedto.DefaultDeleteAppRoleAssignedToOperationOptions()); err != nil {
		return tf.ErrorDiagPathF(err, "id", "Deleting %s: %v", id, err)
	}

	return nil
}
