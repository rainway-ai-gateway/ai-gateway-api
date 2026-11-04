# /entity-types

## 1. 数据模型

```json
{
  "type_name": "dep",
  "description": "一级部门，用于组织架构中的部门层级",
  "level": 1,
  "create_time": 1716883200
}
```

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 |
|------|------|------|----------|
| `type_name` | string | 类型标识 | 全局唯一，1-32字符，仅含小写字母、数字、下划线、连字符 | 必填；类型为 [EntityTypeName](./00-common.md#16-entity-type-名称entitytypename) |
| `description` | string | 类型描述 | 自定义 | 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符 |
| `level` | int | 层级级别 | 取值范围1-5，数字越小级别越高 | 必填；取值范围 1-5 的整数 |
| `create_time` | int64 | 创建时间 | Unix时间戳（秒） |

---

## 2. 接口清单

### 2.1 创建Entity-Type

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 创建Entity类型定义 | - |
| 端点 | /entity-types | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| type_name | string | 类型名 | Y | 全局唯一，1-32字符，仅含小写字母、数字、下划线、连字符 | 必填；类型为 [EntityTypeName](./00-common.md#16-entity-type-名称entitytypename)；全局唯一 |
| description | string | 类型描述 | N | - | 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符 |
| level | int | 层级级别 | Y | 取值范围1-5，数字越小级别越高 | 必填；取值范围 1-5 的整数 |

**HTTP BODY参数示例**

```json
{
    "type_name": "dep",
    "description": "一级部门",
    "level": 1
}
```

**返回数据（Data内容）**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| type_name | string | 类型名 | - |
| description | string | 描述 | - |
| level | int | 层级级别 | - |
| create_time | int64 | 创建时间 | Unix时间戳（秒） |

---

### 2.2 查询Entity-Type列表

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询Entity-Type列表 | - |
| 端点 | /entity-types | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（Query）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| page | int | 页码 | N | **未传分页参数时返回全部匹配记录**；显式分页时默认 1 | 有效值 >0；<=0 按 1 处理 |
| page_size | int | 每页条数 | N | **未传分页参数时返回全部匹配记录**；显式分页时默认 20，最大 100 | 有效范围 1-100；<1 按 20 处理，>100 按 100 处理 |
| id | int64 | 按Entity-Type内部ID过滤 | N | - | - |
| type_name | string | 按类型名过滤 | N | - | - |
| level | int | 按层级级别过滤 | N | - | - |

**返回数据（Data内容）**

返回分页结构：

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| list | []EntityType | Entity-Type列表 | 元素字段同3.2.1创建Entity-Type返回数据 |
| pagination | object | 分页信息 | 包含`page`、`page_size`、`total`；未传分页参数时返回全部匹配记录，`page=1`、`page_size=total`（空集合为 0）；`total` 为满足 `id`/`type_name`/`level` 过滤条件的总条数 |

---

### 2.3 查询单个Entity-Type

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询单个Entity-Type | - |
| 端点 | /entity-types/{type_name} | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| type_name | string | 类型名 | Y | - | 必填；类型为 [EntityTypeName](./00-common.md#16-entity-type-名称entitytypename)；必须引用已存在的 Entity-Type |

**返回数据（Data内容）**

同3.2.1创建Entity-Type返回数据。

---

### 2.4 更新Entity-Type

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 更新Entity-Type描述 | - |
| 端点 | /entity-types/{type_name} | - |
| 版本 | v1 | - |
| method | PATCH | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| type_name | string | 类型名 | Y | - | 必填；类型为 [EntityTypeName](./00-common.md#16-entity-type-名称entitytypename)；必须引用已存在的 Entity-Type |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| description | string | 类型描述 | N | - | 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符；字段规则同 [§1 数据模型](#1-数据模型) |

**返回数据（Data内容）**

同3.2.1创建Entity-Type返回数据。

---

### 2.5 删除Entity-Type

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 删除Entity-Type | - |
| 端点 | /entity-types/{type_name} | - |
| 版本 | v1 | - |
| method | DELETE | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| type_name | string | 类型名 | Y | - | 必填；类型为 [EntityTypeName](./00-common.md#16-entity-type-名称entitytypename)；必须引用已存在的 Entity-Type |

**返回数据（Data内容）**

Data为null。

**约束**：若该Entity-Type已被任何Entity引用，返回ErrNum=409。

---
