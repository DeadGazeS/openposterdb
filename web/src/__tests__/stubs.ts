/** Shared component stubs for unit tests using shadcn-vue Select, Checkbox, and Label. */

export const SelectStub = {
  template: '<div data-slot="select"><slot /></div>',
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
}

export const SelectTriggerStub = {
  template: '<select :id="id" :data-testid="$attrs[\'data-testid\']" :disabled="$parent?.disabled" :value="$parent?.modelValue" @change="$parent?.$emit(\'update:modelValue\', $event.target.value)"><slot /></select>',
  props: ['id', 'class'],
}

export const SelectValueStub = {
  template: '<option disabled value="">{{ placeholder }}</option>',
  props: ['placeholder'],
}

export const SelectContentStub = {
  template: '<slot />',
}

export const SelectItemStub = {
  template: '<option :value="value"><slot /></option>',
  props: ['value'],
}

export const CheckboxStub = {
  template: '<input type="checkbox" :id="id" :checked="modelValue" :disabled="disabled" :data-testid="$attrs[\'data-testid\']" @change="$emit(\'update:modelValue\', $event.target.checked)" />',
  props: ['id', 'modelValue', 'disabled'],
  emits: ['update:modelValue'],
}

export const LabelStub = {
  template: '<label :for="$attrs.for"><slot /></label>',
}

export const ButtonStub = {
  template: '<button :disabled="disabled" :data-testid="$attrs[\'data-testid\']" @click="$emit(\'click\')"><slot /></button>',
  props: ['disabled', 'variant', 'size'],
}

export const shadcnStubs = {
  Select: SelectStub,
  SelectTrigger: SelectTriggerStub,
  SelectValue: SelectValueStub,
  SelectContent: SelectContentStub,
  SelectItem: SelectItemStub,
  Checkbox: CheckboxStub,
  Label: LabelStub,
  Button: ButtonStub,
} as const
