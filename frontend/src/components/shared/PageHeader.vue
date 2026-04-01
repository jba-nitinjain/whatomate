<script setup lang="ts">
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { ArrowLeft } from 'lucide-vue-next'
import type { Component } from 'vue'

const props = defineProps<{
  title: string
  description?: string
  subtitle?: string
  icon?: Component
  iconGradient?: string
  backLink?: string
  breadcrumbs?: Array<{ label: string; href?: string }>
}>()

const subtitleText = computed(() => props.subtitle || props.description)
</script>

<template>
  <header class="border-b border-white/[0.08] light:border-gray-200 bg-[#0a0a0b]/95 light:bg-white/95 backdrop-blur">
    <div class="px-4 py-4 sm:px-6">
      <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div class="flex min-w-0 items-start gap-3">
          <RouterLink v-if="backLink" :to="backLink" class="shrink-0">
            <Button variant="ghost" size="icon" class="h-9 w-9">
              <ArrowLeft class="h-5 w-5" />
            </Button>
          </RouterLink>
          <div
            v-if="icon"
            class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg shadow-lg"
            :class="iconGradient || 'bg-gradient-to-br from-blue-500 to-indigo-600 shadow-blue-500/20'"
          >
            <component :is="icon" class="h-4 w-4 text-white" />
          </div>
          <div class="min-w-0 flex-1">
            <h1 class="text-lg font-semibold text-white light:text-gray-900 sm:text-xl">{{ title }}</h1>
            <template v-if="breadcrumbs?.length">
              <div class="hidden md:block">
                <Breadcrumb>
                  <BreadcrumbList>
                    <template v-for="(crumb, index) in breadcrumbs" :key="index">
                      <BreadcrumbItem>
                        <BreadcrumbLink v-if="crumb.href" :href="crumb.href">
                          {{ crumb.label }}
                        </BreadcrumbLink>
                        <BreadcrumbPage v-else>{{ crumb.label }}</BreadcrumbPage>
                      </BreadcrumbItem>
                      <BreadcrumbSeparator v-if="index < breadcrumbs.length - 1" />
                    </template>
                  </BreadcrumbList>
                </Breadcrumb>
              </div>
            </template>
            <p v-else-if="subtitleText" class="mt-1 text-sm text-white/50 light:text-gray-500">
              {{ subtitleText }}
            </p>
          </div>
        </div>
        <div v-if="$slots.actions" class="flex w-full flex-wrap items-center gap-2 md:w-auto md:justify-end">
          <slot name="actions" />
        </div>
      </div>
    </div>
  </header>
</template>
