<script setup lang="ts">
import type { FormSubmitEvent } from '@nuxt/ui'
import type { NewItem } from '~/utils/api'
import { NEW_ITEM_LIMITS, serverFieldError, toNewItem, validateNewItem } from '~/utils/newItem'

useHead({ title: 'Add item' })

const api = useApi()
const toast = useToast()

const state = reactive<NewItem>({ memberName: '', itemName: '', description: '' })
const error = ref<string>()

const form = useTemplateRef('form')
const itemNameInput = useTemplateRef('itemNameInput')

// UForm disables its inputs while the submit handler runs (loadingAuto), so
// focus can only move to the next field once the form has finished loading.
const focusItemNameWhenReady = ref(false)
watch(() => form.value?.loading, async (loading) => {
  if (loading || !focusItemNameWhenReady.value) {
    return
  }
  focusItemNameWhenReady.value = false
  await nextTick()
  itemNameInput.value?.inputRef?.focus()
})

async function onSubmit(event: FormSubmitEvent<NewItem>) {
  error.value = undefined
  const body = toNewItem(event.data)
  try {
    await api.createItem(body)
    toast.add({
      title: 'Item stored',
      description: `${body.itemName} for ${body.memberName}. Its workflow has started.`,
      color: 'success',
      icon: 'i-lucide-check',
    })
    // Ready for the next item: keep the member, clear the rest.
    state.itemName = ''
    state.description = ''
    form.value?.clear()
    focusItemNameWhenReady.value = true
  }
  catch (err) {
    const apiErr = toApiError(err)
    const fieldErr = apiErr.status === 400 ? serverFieldError(apiErr.message) : undefined
    if (fieldErr) {
      form.value?.setErrors([fieldErr], fieldErr.name)
    }
    else {
      error.value = isUnreachable(apiErr)
        ? 'Could not reach the server. Is the backend running (make server)?'
        : apiErr.message
    }
  }
}
</script>

<template>
  <div class="mx-auto flex max-w-xl flex-col gap-6">
    <div class="flex flex-col gap-2">
      <h1 class="text-2xl font-semibold">
        Add item
      </h1>
      <p class="text-sm text-muted">
        Register an item a member is leaving in the storage area. Each item starts its own workflow; watch it on the dashboard.
      </p>
    </div>

    <UCard>
      <UForm ref="form" :state="state" :validate="validateNewItem" class="flex flex-col gap-4" @submit="onSubmit">
        <UFormField label="Member name" name="memberName" required>
          <UInput v-model="state.memberName" :maxlength="NEW_ITEM_LIMITS.memberName" autocomplete="name" class="w-full" />
        </UFormField>

        <UFormField label="Item name" name="itemName" required>
          <UInput
            ref="itemNameInput"
            v-model="state.itemName"
            :maxlength="NEW_ITEM_LIMITS.itemName"
            placeholder="e.g. Bike frame"
            class="w-full"
          />
        </UFormField>

        <UFormField label="Description" name="description" hint="Optional">
          <UTextarea
            v-model="state.description"
            :maxlength="NEW_ITEM_LIMITS.description"
            :rows="3"
            autoresize
            placeholder="Anything that helps identify it"
            class="w-full"
          />
        </UFormField>

        <UAlert v-if="error" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="error" />

        <div class="flex items-center justify-between gap-4">
          <ULink to="/dashboard" class="text-sm">
            Go to the dashboard
          </ULink>
          <UButton type="submit" label="Store item" icon="i-lucide-package-plus" loading-auto />
        </div>
      </UForm>
    </UCard>
  </div>
</template>
