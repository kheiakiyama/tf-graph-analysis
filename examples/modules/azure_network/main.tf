# Resource Group
resource "azurerm_resource_group" "rg" {
  name     = var.resource_group_name
  location = var.location
  tags = {
    Environment = var.environment
    ManagedBy   = "tf-graph-analysis-test"
  }
}

# Virtual Network (depends on Resource Group)
resource "azurerm_virtual_network" "vnet" {
  name                = "${var.environment}-vnet"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
  address_space       = ["10.0.0.0/16"]

  tags = azurerm_resource_group.rg.tags
}

# Subnets (depend on Virtual Network)
resource "azurerm_subnet" "frontend" {
  name                 = "frontend-subnet"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.vnet.name
  address_prefixes     = ["10.0.1.0/24"]
}

resource "azurerm_subnet" "backend" {
  name                 = "backend-subnet"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.vnet.name
  address_prefixes     = ["10.0.2.0/24"]
}

resource "azurerm_subnet" "database" {
  name                 = "database-subnet"
  resource_group_name  = azurerm_resource_group.rg.name
  virtual_network_name = azurerm_virtual_network.vnet.name
  address_prefixes     = ["10.0.3.0/24"]
}

# Network Security Groups (parallel branch from Resource Group)
resource "azurerm_network_security_group" "frontend" {
  name                = "${var.environment}-frontend-nsg"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
  tags                = azurerm_resource_group.rg.tags
}

resource "azurerm_network_security_group" "backend" {
  name                = "${var.environment}-backend-nsg"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
  tags                = azurerm_resource_group.rg.tags
}

resource "azurerm_network_security_group" "database" {
  name                = "${var.environment}-database-nsg"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
  tags                = azurerm_resource_group.rg.tags
}

# Network Security Rules (depend on NSGs)
resource "azurerm_network_security_rule" "frontend_http" {
  name                        = "AllowHTTP"
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "80"
  source_address_prefix       = "*"
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.rg.name
  network_security_group_name = azurerm_network_security_group.frontend.name
}

resource "azurerm_network_security_rule" "frontend_https" {
  name                        = "AllowHTTPS"
  priority                    = 110
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "443"
  source_address_prefix       = "*"
  destination_address_prefix  = "*"
  resource_group_name         = azurerm_resource_group.rg.name
  network_security_group_name = azurerm_network_security_group.frontend.name
}

resource "azurerm_network_security_rule" "backend_internal" {
  name                        = "AllowFrontendToBackend"
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "8080"
  source_address_prefix       = "10.0.1.0/24"
  destination_address_prefix  = "10.0.2.0/24"
  resource_group_name         = azurerm_resource_group.rg.name
  network_security_group_name = azurerm_network_security_group.backend.name
}

resource "azurerm_network_security_rule" "database_internal" {
  name                        = "AllowBackendToDatabase"
  priority                    = 100
  direction                   = "Inbound"
  access                      = "Allow"
  protocol                    = "Tcp"
  source_port_range           = "*"
  destination_port_range      = "5432"
  source_address_prefix       = "10.0.2.0/24"
  destination_address_prefix  = "10.0.3.0/24"
  resource_group_name         = azurerm_resource_group.rg.name
  network_security_group_name = azurerm_network_security_group.database.name
}

# Route Table (parallel branch from Resource Group)
resource "azurerm_route_table" "rt" {
  name                = "${var.environment}-route-table"
  resource_group_name = azurerm_resource_group.rg.name
  location            = azurerm_resource_group.rg.location
  tags                = azurerm_resource_group.rg.tags
}

resource "azurerm_route" "local" {
  name                = "LocalVNetRoute"
  resource_group_name = azurerm_resource_group.rg.name
  route_table_name    = azurerm_route_table.rt.name
  address_prefix      = "10.0.0.0/16"
  next_hop_type       = "VnetLocal"
}

# Subnet - NSG Associations (converges Subnets and NSGs)
resource "azurerm_subnet_network_security_group_association" "frontend" {
  subnet_id                 = azurerm_subnet.frontend.id
  network_security_group_id = azurerm_network_security_group.frontend.id
  depends_on = [
    azurerm_network_security_rule.frontend_http,
    azurerm_network_security_rule.frontend_https
  ]
}

resource "azurerm_subnet_network_security_group_association" "backend" {
  subnet_id                 = azurerm_subnet.backend.id
  network_security_group_id = azurerm_network_security_group.backend.id
  depends_on = [
    azurerm_network_security_rule.backend_internal
  ]
}

resource "azurerm_subnet_network_security_group_association" "database" {
  subnet_id                 = azurerm_subnet.database.id
  network_security_group_id = azurerm_network_security_group.database.id
  depends_on = [
    azurerm_network_security_rule.database_internal
  ]
}

# Subnet - Route Table Associations (converges Subnets and Route Table)
resource "azurerm_subnet_route_table_association" "frontend" {
  subnet_id      = azurerm_subnet.frontend.id
  route_table_id = azurerm_route_table.rt.id
  depends_on     = [azurerm_route.local]
}

resource "azurerm_subnet_route_table_association" "backend" {
  subnet_id      = azurerm_subnet.backend.id
  route_table_id = azurerm_route_table.rt.id
  depends_on     = [azurerm_route.local]
}
