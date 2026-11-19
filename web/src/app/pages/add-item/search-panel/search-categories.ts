import { ItemForm } from '../../../models';
import { ItemLookupCategory } from '../../../services/item-lookup.service';

export interface SearchCategoryConfig {
    value: ItemLookupCategory;
    label: string;
    description: string;
    inputLabel: string;
    placeholder: string;
    itemType: ItemForm['itemType'];
    disabled?: boolean;
}

export const SEARCH_CATEGORIES: SearchCategoryConfig[] = [
    {
        value: 'book',
        label: 'Book',
        description: 'Search by ISBN or keyword to auto-fill book details.',
        inputLabel: 'Search for books',
        placeholder: 'ISBN or title keyword',
        itemType: 'book',
    },
    {
        value: 'game',
        label: 'Game',
        description: 'Look up tabletop or video releases by UPC or title.',
        inputLabel: 'Search for games',
        placeholder: 'UPC or title keyword',
        itemType: 'game',
        disabled: true,
    },
    {
        value: 'movie',
        label: 'Movie',
        description: 'Use UPC or keywords to find film metadata.',
        inputLabel: 'Search for movies',
        placeholder: 'UPC or title keyword',
        itemType: 'movie',
        disabled: true,
    },
    {
        value: 'music',
        label: 'Music',
        description: 'Find album details with UPC or artist keywords.',
        inputLabel: 'Search for music',
        placeholder: 'UPC or title keyword',
        itemType: 'music',
        disabled: true,
    },
];

export function getSearchCategory(value: ItemLookupCategory | null): SearchCategoryConfig {
    return SEARCH_CATEGORIES.find((category) => category.value === value) ?? SEARCH_CATEGORIES[0];
}
