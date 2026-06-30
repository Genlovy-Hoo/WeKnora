<!-- AI-generated: 自定义 HTTP 工具编辑表单。新建/编辑/测试调用一体。
     采用项目已验证的 t-dialog + t-form 写法（对照 CreateTenantDialog）：:visible/@update:visible + reactive form。 -->
<template>
  <t-dialog
    :visible="props.visible"
    @update:visible="(v: boolean) => emit('update:visible', v)"
    :header="isEdit ? t('tools.editor.editTitle') : t('tools.editor.createTitle')"
    width="70%"
    top="6vh"
    dialog-class-name="tool-editor-dialog"
    :footer="false"
    :close-on-overlay-click="false"
    @close="onClose"
  >
    <t-form ref="formRef" :data="formData" :rules="rules" label-align="top" @submit.prevent="onSubmit">
      <t-form-item :label="t('tools.editor.name')" name="name">
        <t-input
          v-model="formData.name"
          :placeholder="t('tools.editor.namePlaceholder')"
          :disabled="isEdit"
        />
        <template #help>{{ t('tools.editor.nameHelp') }}</template>
      </t-form-item>

      <t-form-item :label="t('tools.editor.displayName')" name="display_name">
        <t-input v-model="formData.display_name" :placeholder="t('tools.editor.displayNamePlaceholder')" />
      </t-form-item>

      <t-form-item :label="t('tools.editor.description')" name="description">
        <t-textarea v-model="formData.description" :autosize="{ minRows: 2, maxRows: 4 }" />
      </t-form-item>

      <t-divider align="left">{{ t('tools.editor.httpSection') }}</t-divider>

      <div class="http-row">
        <t-form-item :label="t('tools.editor.method')" name="method" style="width: 120px">
          <t-select v-model="formData.http_config.method">
            <t-option v-for="m in methods" :key="m" :value="m" :label="m" />
          </t-select>
        </t-form-item>
        <t-form-item :label="t('tools.editor.urlTemplate')" name="url_template" style="flex: 1">
          <t-input v-model="formData.http_config.url_template" placeholder="https://api.example.com/v1/{id}" />
        </t-form-item>
      </div>

      <t-form-item :label="t('tools.editor.headers')" name="headers">
        <div class="kv-list">
          <div v-for="(item, idx) in headerRows" :key="idx" class="kv-row">
            <t-input v-model="item.key" placeholder="Header" />
            <t-input v-model="item.value" placeholder="Value" />
            <t-button theme="danger" variant="text" @click="headerRows.splice(idx, 1)">
              <t-icon name="delete" />
            </t-button>
          </div>
          <t-button theme="default" variant="dashed" block @click="headerRows.push({ key: '', value: '' })">
            <template #icon><t-icon name="add" /></template>
            {{ t('tools.editor.addHeader') }}
          </t-button>
        </div>
      </t-form-item>

      <t-form-item v-if="formData.http_config.method !== 'GET'" :label="t('tools.editor.bodyTemplate')" name="body_template">
        <t-textarea v-model="formData.http_config.body_template" :autosize="{ minRows: 3, maxRows: 8 }" placeholder='{"query": "{q}", "limit": 10}' />
        <template #help>{{ t('tools.editor.bodyHelp') }}</template>
      </t-form-item>

      <t-divider align="left">{{ t('tools.editor.authSection') }}</t-divider>

      <div class="http-row">
        <t-form-item :label="t('tools.editor.authType')" style="width: 140px">
          <t-select v-model="formData.http_config.auth.auth_type">
            <t-option value="" label="None" />
            <t-option value="bearer" label="Bearer" />
            <t-option value="apikey" label="API Key" />
            <t-option value="basic" label="Basic" />
          </t-select>
        </t-form-item>
        <t-form-item
          v-if="formData.http_config.auth.auth_type === 'apikey'"
          :label="t('tools.editor.authHeader')"
          style="flex: 1"
        >
          <t-input v-model="formData.http_config.auth.header" placeholder="X-API-Key" />
        </t-form-item>
      </div>

      <t-form-item
        v-if="formData.http_config.auth.auth_type"
        :label="formData.http_config.auth.auth_type === 'basic' ? t('tools.editor.basicToken') : t('tools.editor.token')"
      >
        <t-input
          v-model="formData.http_config.auth.token"
          type="password"
          :placeholder="isEdit && authConfigured && !formData.http_config.auth.token ? t('tools.editor.tokenKept') : t('tools.editor.tokenPlaceholder')"
        />
        <template v-if="isEdit && authConfigured" #help>{{ t('tools.editor.tokenKeepHint') }}</template>
      </t-form-item>

      <t-divider align="left">{{ t('tools.editor.advancedSection') }}</t-divider>

      <t-form-item :label="t('tools.editor.parametersSchema')" name="parameters_schema">
        <t-textarea
          v-model="parametersSchemaText"
          :autosize="{ minRows: 4, maxRows: 10 }"
          placeholder='{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}'
        />
        <template #help>{{ t('tools.editor.schemaHelp') }}</template>
      </t-form-item>

      <div class="http-row">
        <t-form-item :label="t('tools.editor.responseExtract')" style="flex: 1">
          <t-input v-model="formData.http_config.response_extract" placeholder="$.results (optional)" />
        </t-form-item>
        <t-form-item :label="t('tools.editor.timeout')" style="width: 120px">
          <t-input-number v-model="formData.http_config.timeout_sec" :min="1" :max="300" />
        </t-form-item>
      </div>

      <t-form-item :label="t('tools.editor.requireApproval')" name="require_approval">
        <t-switch v-model="formData.require_approval" />
        <template #help>{{ t('tools.editor.requireApprovalHelp') }}</template>
      </t-form-item>

      <t-form-item :label="t('tools.editor.enabled')" name="enabled">
        <t-switch v-model="formData.enabled" />
      </t-form-item>

      <t-divider />

      <div class="test-section">
        <div class="test-section__header">
          <span>{{ t('tools.editor.testTitle') }}</span>
          <t-button theme="primary" variant="outline" :loading="testing" @click="onTest">
            <template #icon><t-icon name="refresh" /></template>
            {{ t('tools.editor.testBtn') }}
          </t-button>
        </div>
        <t-textarea
          v-model="testArgsText"
          :autosize="{ minRows: 2, maxRows: 6 }"
          :placeholder="t('tools.editor.testArgsPlaceholder')"
        />
        <div v-if="testResult" class="test-result" :class="{ 'test-result--err': !testResult.success }">
          <pre>{{ testResult.success ? testResult.output : testResult.error }}</pre>
        </div>
      </div>

      <div class="form-footer">
        <t-button theme="default" variant="base" @click="emit('update:visible', false)">{{ t('common.cancel') }}</t-button>
        <t-button theme="primary" :loading="saving" @click="onSubmit">{{ t('common.save') }}</t-button>
      </div>
    </t-form>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin, type FormInstanceFunctions, type FormRule } from 'tdesign-vue-next'
import {
  createCustomTool,
  updateCustomTool,
  testCustomTool,
  type CustomTool,
  type CustomToolRequest,
} from '@/api/tool'

const props = defineProps<{
  visible: boolean
  libraryId: string
  editTool?: CustomTool | null
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'saved'): void
}>()

const { t } = useI18n()

const isEdit = computed(() => !!props.editTool)
const authConfigured = computed(() => !!props.editTool?.http_config?.auth?.configured)
const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']

interface AuthState {
  auth_type: string
  token: string
  header: string
}
interface HttpConfigState {
  method: string
  url_template: string
  body_template: string
  auth: AuthState
  response_extract: string
  timeout_sec: number
}
interface FormState {
  name: string
  display_name: string
  description: string
  http_config: HttpConfigState
  enabled: boolean
  require_approval: boolean
}

function emptyForm(): FormState {
  return {
    name: '',
    display_name: '',
    description: '',
    http_config: {
      method: 'GET',
      url_template: '',
      body_template: '',
      auth: { auth_type: '', token: '', header: '' },
      response_extract: '',
      timeout_sec: 30,
    },
    enabled: true,
    require_approval: false,
  }
}

const formRef = ref<FormInstanceFunctions | null>(null)
const formData = reactive<FormState>(emptyForm())
const headerRows = ref<{ key: string; value: string }[]>([])
const parametersSchemaText = ref('{}')
const testArgsText = ref('{}')
const testResult = ref<{ success: boolean; output?: string; error?: string } | null>(null)
const saving = ref(false)
const testing = ref(false)

const rules: Record<string, FormRule[]> = {
  name: [{ required: true, message: t('common.required'), type: 'error' }],
  description: [{ required: true, message: t('common.required'), type: 'error' }],
  url_template: [{ required: true, message: t('common.required'), type: 'error' }],
}

watch(
  () => props.visible,
  (v) => {
    if (!v) return
    testResult.value = null
    testArgsText.value = '{}'
    if (props.editTool) {
      const tt = props.editTool
      Object.assign(formData, {
        name: tt.name,
        display_name: tt.display_name || '',
        description: tt.description,
        http_config: {
          method: tt.http_config.method || 'GET',
          url_template: tt.http_config.url_template,
          body_template: tt.http_config.body_template || '',
          auth: {
            auth_type: tt.http_config.auth?.auth_type || '',
            header: tt.http_config.auth?.header || '',
            token: '',
          },
          response_extract: tt.http_config.response_extract || '',
          timeout_sec: tt.http_config.timeout_sec || 30,
        },
        enabled: tt.enabled,
        require_approval: tt.require_approval,
      })
      headerRows.value = Object.entries(tt.http_config.headers || {}).map(([key, value]) => ({ key, value: String(value) }))
      parametersSchemaText.value = tt.parameters_schema ? JSON.stringify(tt.parameters_schema, null, 2) : '{}'
    } else {
      Object.assign(formData, emptyForm())
      headerRows.value = []
      parametersSchemaText.value = '{}'
    }
  },
)

function buildRequest(): CustomToolRequest | null {
  const headers: Record<string, string> = {}
  for (const r of headerRows.value) {
    if (r.key.trim()) headers[r.key.trim()] = r.value
  }
  let schema: Record<string, any> | undefined
  try {
    schema = parametersSchemaText.value.trim() ? JSON.parse(parametersSchemaText.value) : undefined
  } catch {
    MessagePlugin.error('参数 Schema 不是合法 JSON')
    return null
  }
  const auth = { ...formData.http_config.auth }
  // 编辑时若 token 留空且原已配置，则不传 auth（保留服务端原值）
  if (isEdit.value && !auth.token && authConfigured.value) {
    return {
      name: formData.name,
      display_name: formData.display_name,
      description: formData.description,
      parameters_schema: schema,
      http_config: {
        method: formData.http_config.method,
        url_template: formData.http_config.url_template,
        headers,
        body_template: formData.http_config.body_template,
        response_extract: formData.http_config.response_extract,
        timeout_sec: formData.http_config.timeout_sec,
      },
      enabled: formData.enabled,
      require_approval: formData.require_approval,
    }
  }
  return {
    name: formData.name,
    display_name: formData.display_name,
    description: formData.description,
    parameters_schema: schema,
    http_config: {
      method: formData.http_config.method,
      url_template: formData.http_config.url_template,
      headers,
      body_template: formData.http_config.body_template,
      auth: auth.auth_type ? auth : undefined,
      response_extract: formData.http_config.response_extract,
      timeout_sec: formData.http_config.timeout_sec,
    },
    enabled: formData.enabled,
    require_approval: formData.require_approval,
  }
}

async function onSubmit() {
  const req = buildRequest()
  if (!req) return
  saving.value = true
  try {
    if (isEdit.value && props.editTool) {
      await updateCustomTool(props.editTool.id, req)
    } else {
      await createCustomTool(props.libraryId, req)
    }
    MessagePlugin.success('保存成功')
    emit('saved')
    emit('update:visible', false)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function onTest() {
  if (!props.editTool) {
    MessagePlugin.warning('请先保存工具再测试')
    return
  }
  let args: Record<string, any> = {}
  try {
    args = testArgsText.value.trim() ? JSON.parse(testArgsText.value) : {}
  } catch {
    MessagePlugin.error('测试入参不是合法 JSON')
    return
  }
  testing.value = true
  testResult.value = null
  try {
    const res: any = await testCustomTool(props.editTool.id, args)
    testResult.value = res.data
  } catch (e: any) {
    testResult.value = { success: false, error: e?.message || '测试失败' }
  } finally {
    testing.value = false
  }
}

function onClose() {
  testResult.value = null
}
</script>

<style lang="less" scoped>
.http-row {
  display: flex;
  gap: 12px;
}
.kv-list {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.kv-row {
  display: flex;
  gap: 8px;
}
.test-section {
  margin-bottom: 16px;
  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
    font-weight: 600;
  }
}
.test-result {
  margin-top: 8px;
  padding: 10px;
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
  max-height: 220px;
  overflow: auto;
  pre {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    font-size: 12px;
  }
  &--err {
    background: var(--td-error-color-1);
    color: var(--td-error-color-6);
  }
}
.form-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  position: sticky;
  bottom: 0;
}
</style>

<style lang="less">
// Dialog 被 teleport 到 body，scoped 样式够不着，需用非 scoped 样式约束。
// 浏览器原生拖拽调整大小：右下角拖拽手柄，内部 flex 自适应。
.tool-editor-dialog {
  max-height: 88vh;
  resize: both;
  overflow: hidden;
  min-width: 480px;
  min-height: 320px;
  max-width: 96vw;
  display: flex;
  flex-direction: column;

  .t-dialog__body {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    padding-bottom: 0;
  }

  .t-form {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    padding-bottom: 16px;
  }
}
</style>
