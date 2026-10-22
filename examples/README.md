# Test Examples for `tf-graph-analysis`

This directory contains reference Terraform code used to test and demonstrate `tf-graph-analysis` across both supported execution patterns:

1. **Pattern A: Local Execution** (`examples/azure-local`) - Ran on the GitHub Actions runner.
2. **Pattern B: Remote Execution** (`examples/azure-tfc`) - Ran on HCP Terraform (Terraform Cloud).

---

## Architecture: Zero-Cost Azure Network

The shared module (`examples/modules/azure_network`) deploys a realistic, multi-tiered virtual network architecture that incurs **$0.00 / month in Azure charges**:

- **Resource Group** (`azurerm_resource_group`) - Free
- **Virtual Network** (`azurerm_virtual_network`) - Free
- **Subnets** (Frontend, Backend, Database) (`azurerm_subnet`) - Free
- **Network Security Groups & Rules** (`azurerm_network_security_group`, `azurerm_network_security_rule`) - Free
- **Route Table & Routes** (`azurerm_route_table`, `azurerm_route`) - Free
- **Subnet Associations** (`azurerm_subnet_network_security_group_association`, `azurerm_subnet_route_table_association`) - Free

### Dependency DAG Structure

```mermaid
graph TD
    RG[Resource Group] --> VNET[Virtual Network]
    RG --> NSG_APP[NSG: App]
    RG --> NSG_DB[NSG: DB]
    RG --> RT[Route Table]

    VNET --> SUB_APP[Subnet: Frontend]
    VNET --> SUB_DB[Subnet: Backend]
    VNET --> SUB_DATA[Subnet: Database]

    NSG_APP --> RULE_HTTP[Rule: HTTP]
    NSG_APP --> RULE_HTTPS[Rule: HTTPS]

    SUB_APP --> ASSOC_APP_NSG[Subnet-NSG Assoc: Frontend]
    NSG_APP --> ASSOC_APP_NSG

    SUB_DB --> ASSOC_DB_NSG[Subnet-NSG Assoc: Backend]
    NSG_DB --> ASSOC_DB_NSG

    SUB_APP --> ASSOC_APP_RT[Subnet-RT Assoc: Frontend]
    RT --> ASSOC_APP_RT
```

This structure intentionally contains both parallel branches (NSGs and Route Tables created concurrently) and sequential dependency chains (VNet -> Subnets -> Associations), providing an ideal benchmark for Critical Path Method (CPM) analysis.

---

## Directory Structure

```text
examples/
├── modules/
│   └── azure_network/        # Shared module with zero-cost network resources
│       ├── main.tf
│       ├── variables.tf
│       └── outputs.tf
├── azure-local/              # Pattern A: Local execution on GitHub Actions runner
│   ├── main.tf
│   ├── providers.tf
│   └── variables.tf
└── azure-tfc/                # Pattern B: Remote execution via HCP Terraform
    ├── main.tf
    ├── providers.tf          # Contains cloud {} block
    └── variables.tf
```

---

## Security & Sensitive Information in Public Repositories

When integrating with public GitHub repositories:
- **No hardcoded secrets**: All Azure Subscription IDs, Tenant IDs, and Client IDs are supplied via GitHub Repository Secrets (`secrets.AZURE_*`) and masked automatically.
- **Plan logs not dumped to console**: In the test workflow, `terraform plan` logs are redirected to a local file (`tf_plan.log`) and read only by the analyzer, preventing Azure resource IDs from being printed in GitHub Actions job output.
- **Fork protection**: Workflows test only when repository secrets are present, ensuring external fork PRs run securely without errors.
