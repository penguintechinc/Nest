# Arctic DBM Design Review Document

# Context

For database access, there are three “planes” which you have to solve for:

- Dataplane \- how we query and update the data itself  
- Management \- how we grant access on a granular level using RBAC  
- Proxy \- how we filter queries coming in based on the request itself and user authorization

There are few problems we are trying to solve here:

- Limiting database server nodes  
- Limiting user access to a table and eventually on a column basis  
- Try to filter out common SQL attacks before they even hit the database itself  
- Allow microservices to talk on HTTP like they typically do with each other  
- Make it easy to migrate from DB libraries to this with a client example and a legacy adapter

The idea came when there was two environments not directly connected which needed to talk to each other. Each environment only had a web reverse proxy. 

# Architecture

## Data Flow Diagram

![][image1]  
An optional LoadBalancer is called out here. For our main thoughts, ArticDBM would be in its own namespace within Kubernetes. Databases would be in a separate one which will only communicate with ArticDBM Namespace. This allows for monitoring in between namespaces to be available if necessary via network policies.

# Arctic DBM Server

This is a server which tackles the API to DB and DB Proxy/Filtering.   
It contains the mapping of DBs, users, application, etc. and makes DB connections more secure as well as web based.

## Authentication

For MVP \- authentication will be basic user authentication. On server standup, through environment variables, a root user will be created. It can then be used to create other users.

## Roles

Administrator \- Can update and user, database, etc.  
Developer \- Can create and update their own databases only  
Reporter \- Can query databases but not update nor create them

## Arctic’s own DB Design

This is the design which ArticDB will use to store it’s own data, by default in SQLITE  
Database: ArticDB

| Table | Column | PyDAL Type | Notes |
| ----- | ----- | ----- | ----- |
| Admin | Username |  |  |
|  | Password |  |  |
|  | IP | str | CIDR format |
|  | Permissions | RO / Limited / Admin |  |
| Databases | Name |  |  |
|  | Type |  |  |
|  | User |  |  |
|  | options |  |  |
| Tables | Name |  |  |
|  | Db |  |  |
|  | User | if blank \- default DB user |  |
|  | columnDefs | dictionary/json |  |
| Hosts | dbHost | str |  |
|  | dbPort | int |  |
|  | dbType | str |  |
|  | dbUser | str |  |
|  | dbPassword | password |  |
|  | dbTLS | boolean | FUTURE |
|  | dbOptions | List: string | Any additional options for the DB (ie: ISO) \- FUTURE |

## API Design

### ADMIN

```
/admin/user/add (Adds or updates user)  
{ username, password, role }

/admin/user/del (deletes user)  
{ username }
```

### DB

```
/db/add (Create/Update DB)  
{ dbname:str, dbType:str, user:liststr, options\*:json }

/db/del (delete DB)  
{ dbname:str }

/db/table/add (Create / update table)  
{dbName:str, tbName:str, columns:json{name:pydalType}, user\*:liststr }
```

### DATA 

```
/{{dbname}}/{tableName}/query (pull data)  
{columns:liststr, condition: str}

/{{dbname}}/{tableName}/update (Update existing)  
{columns:liststr, condition: str}

/{{dbname}}/{tableName}/insert  
Json{column:data…}
```



### DBHOST

```
/host/add (Create and update)  
{dbFQDN, dbPort, dbUser, dbPass, dbPyDALType, dbTLS,}

/host/del (Delete)  
{dbFQDN}
```

# ArticDBClient

This would be a python client which makes it easy to request data from databases using a simple yaml configuration file. These would utilize requests library.

There would be 3 clients in total:

- Embedded python library  
- Legacy Network Adapter  
- Initialization Client

## Config Examples

### Basic Config

```yaml
---
ArticHost: somedomain.localnet  
ArticPort: 38306  
ArticDB: somedb  
API: REST
```

### Initialization Config

```yaml
---
ArticHost: somedomain.localnet
ArticPort: 38306
ArticDB: somedb
API: REST
Initialize:
  Tables:
    Sometable:
      Columns:
        - Name: str
        - Address: str
      Users:
        - Someuser
        - Someuser2
```


# Future Vision

## gRPC

Add an additional gRPC endpoint on the server and add it to the clients as preferred method. This will increase speed and somewhat protect the data across the wire better too.

## Authentication

Eventually we want to allow for LDAP / SAML / OAUTH2. We also want to enable user filtering on a row and column basis 

[image1]: <data:image/png;base64,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>